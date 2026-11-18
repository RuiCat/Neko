// applyPlan.go turns one settlement write set into a validated batch. Every row
// is applied to a deep copy of the touched entities first; only when the whole
// batch is valid are the real entities written, and a write an entity rejects
// rolls the already written rows back to their pre-batch values.
package host

import (
	"fmt"
	"strings"

	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/settle"
	"github.com/RuiCat/Neko/internal/flow/spec"
)

// KEY_ITEM_DESCRIPTION is the material kernel's 描述 field, carried over from the
// K1 produce item so a created material keeps the corpus text.
const KEY_ITEM_DESCRIPTION = "描述"

// applyPlan is one settlement batch: the entities it touches with their
// pre-batch snapshot and working copy, plus the material entities it creates.
type applyPlan struct {
	store   *entity.Store
	targets map[string]*targetWrites
	order   []string
	creates []pendingCreate
}

// targetWrites is one touched entity. before is the rollback source and work is
// the mutable copy the rows are applied to; both are independent deep copies
// taken under the entity's lock.
type targetWrites struct {
	entity  *entity.Entity
	before  map[string]any
	work    map[string]any
	paths   []string
	touched map[string]struct{}
}

// pendingCreate is one material entity a produce row creates at commit time.
type pendingCreate struct {
	id       string
	typeName string
	data     map[string]any
}

// appliedWrite is one entity write that commit performed, remembered so a later
// refusal can restore it.
type appliedWrite struct {
	target   *targetWrites
	path     string
	prior    any
	hasPrior bool
}

// newApplyPlan validates every row against working copies. No entity is written
// when it returns an error.
func newApplyPlan(store *entity.Store, changes []settle.Change) (*applyPlan, error) {
	if store == nil {
		return nil, fmt.Errorf("no entity store")
	}
	plan := &applyPlan{
		store:   store,
		targets: make(map[string]*targetWrites, len(changes)),
	}
	for index, change := range changes {
		if err := plan.add(change); err != nil {
			return nil, fmt.Errorf("change[%d]: %w", index, err)
		}
	}
	return plan, nil
}

// add dispatches one settlement row by its operation.
func (p *applyPlan) add(change settle.Change) error {
	switch change.Op {
	case settle.OP_SPEND, settle.OP_EFFECT:
		return p.addField(change)
	case settle.OP_CONSUME, settle.OP_PRODUCE:
		return p.addItem(change)
	default:
		return fmt.Errorf("unknown change op %q", change.Op)
	}
}

// addField applies one actor field row (a trade request or an effect) to the
// actor's working copy.
func (p *applyPlan) addField(change settle.Change) error {
	if change.Path == "" {
		return fmt.Errorf("empty write path")
	}
	ent := p.store.Get(change.Entity)
	if ent == nil {
		return fmt.Errorf("entity %q not found", change.Entity)
	}
	target := p.target(ent)
	value, err := target.fieldValue(change)
	if err != nil {
		return fmt.Errorf("%s: %w", change.Path, err)
	}
	if err := writePath(target.work, change.Path, value); err != nil {
		return err
	}
	target.record(change.Path)
	return nil
}

// fieldValue converts one row into the value written to the field. An add row
// reads the current value first: an absent row counts as zero, matching the
// settlement report semantics.
func (t *targetWrites) fieldValue(change settle.Change) (any, error) {
	switch change.Kind {
	case spec.EFFECT_KIND_SET:
		return change.Value, nil
	case spec.EFFECT_KIND_ADD:
		return t.added(change.Path, change.Value)
	case spec.EFFECT_KIND_TEXT:
		text, ok := change.Value.(string)
		if !ok {
			return nil, fmt.Errorf("text wants a string, got %T", change.Value)
		}
		return text, nil
	case spec.EFFECT_KIND_BOOL:
		flag, ok := change.Value.(bool)
		if !ok {
			return nil, fmt.Errorf("bool wants a boolean, got %T", change.Value)
		}
		return flag, nil
	default:
		return nil, fmt.Errorf("unknown effect kind %q", change.Kind)
	}
}

// added returns the current value plus delta. The current value comes from the
// working copy, falling back to the entity (template defaults included).
func (t *targetWrites) added(path string, delta any) (int, error) {
	step, err := intValue(delta, fmt.Sprintf("%s delta", path))
	if err != nil {
		return 0, err
	}
	current := t.read(path)
	if current == nil {
		return step, nil
	}
	base, err := intValue(current, path)
	if err != nil {
		return 0, err
	}
	return base + step, nil
}

// addItem applies one item quantity delta. A consume needs the material entity
// to exist; a produce creates it when it does not, and the create is queued
// until commit so a later refusal creates nothing.
func (p *applyPlan) addItem(change settle.Change) error {
	if change.Entity == "" {
		return fmt.Errorf("item name is empty")
	}
	delta, err := intValue(change.Value, fmt.Sprintf("item %q quantity", change.Entity))
	if err != nil {
		return err
	}
	if material := p.material(change.Entity, change.Scene); material != nil {
		return p.adjustQuantity(material, delta)
	}
	switch change.Op {
	case settle.OP_CONSUME:
		return fmt.Errorf("item %q in scene %q not found", change.Entity, change.Scene)
	case settle.OP_PRODUCE:
		return p.queueCreation(change, delta)
	default:
		return fmt.Errorf("item %q: %s needs an existing material entity", change.Entity, change.Op)
	}
}

// adjustQuantity moves one material's 数量 by delta and refuses a negative
// result; the K4 report already rejects a short consume, so this guard keeps the
// store honest for a hand-built batch.
func (p *applyPlan) adjustQuantity(material *entity.Entity, delta int) error {
	target := p.target(material)
	current := target.read(settle.PATH_ITEM_QUANTITY)
	if current == nil {
		current = 1
	}
	base, err := intValue(current, fmt.Sprintf("item %q %s", material.ID, settle.PATH_ITEM_QUANTITY))
	if err != nil {
		return err
	}
	total := base + delta
	if total < 0 {
		return fmt.Errorf("item %q: %d - %d leaves a negative quantity", material.ID, base, -delta)
	}
	if err := writePath(target.work, settle.PATH_ITEM_QUANTITY, total); err != nil {
		return err
	}
	target.record(settle.PATH_ITEM_QUANTITY)
	return nil
}

// queueCreation validates one material creation and defers it to commit.
func (p *applyPlan) queueCreation(change settle.Change, quantity int) error {
	if quantity < 0 {
		return fmt.Errorf("item %q: negative produce quantity %d", change.Entity, quantity)
	}
	if change.Item == nil {
		return fmt.Errorf("item %q: produce row carries no item template", change.Entity)
	}
	typeName := change.Item.Type
	if typeName == "" {
		return fmt.Errorf("item %q: produce template has no type", change.Entity)
	}
	if !p.store.HasTemplate(typeName) {
		return fmt.Errorf("item %q: template %q is missing", change.Entity, typeName)
	}
	id, err := materialID(change.Entity, change.Scene)
	if err != nil {
		return err
	}
	if p.store.Get(id) != nil {
		return fmt.Errorf("material id %q is already taken", id)
	}
	for _, pending := range p.creates {
		if pending.id == id {
			return fmt.Errorf("material %q is queued twice", id)
		}
	}
	data := map[string]any{
		condition.KEY_ITEM_NAME:   change.Entity,
		condition.KEY_ITEM_SCENE:  change.Scene,
		settle.PATH_ITEM_QUANTITY: quantity,
		KEY_ITEM_DESCRIPTION:      change.Item.Description,
	}
	p.creates = append(p.creates, pendingCreate{id: id, typeName: typeName, data: data})
	return nil
}

// material resolves one material entity by (name, scene). An empty scene matches
// the first entity of that name, the same rule K4 uses for consume rows.
func (p *applyPlan) material(name, scene string) *entity.Entity {
	for _, ent := range p.store.ListAll() {
		if !entity.IsSubtypeOf(ent.Type, MATERIAL_TYPE) {
			continue
		}
		itemName, ok := ent.GetOrNone(condition.KEY_ITEM_NAME).(string)
		if !ok || itemName != name {
			continue
		}
		itemScene, _ := ent.GetOrNone(condition.KEY_ITEM_SCENE).(string)
		if scene == "" || itemScene == scene {
			return ent
		}
	}
	return nil
}

// materialID derives the deterministic entity id of a created material. The id
// must pass entity.ValidateID, which is why the forbidden characters are checked
// here instead of at Create time.
func materialID(name, scene string) (string, error) {
	id := MATERIAL_TYPE + ":" + scene + "@" + name
	if strings.ContainsAny(id, ".[] ") {
		return "", fmt.Errorf("material %q in scene %q: id %q contains a forbidden character", name, scene, id)
	}
	return id, nil
}

// target returns the tracked working state of one entity, taking the pre-batch
// snapshot and cloning it into a working copy on first touch. A second row for
// the same entity reuses the same working copy so a set followed by an add sees
// the set.
func (p *applyPlan) target(ent *entity.Entity) *targetWrites {
	if existing, ok := p.targets[ent.ID]; ok {
		return existing
	}
	before := ent.Snapshot()
	target := &targetWrites{
		entity:  ent,
		before:  before,
		work:    cloneData(before),
		touched: map[string]struct{}{},
	}
	p.targets[ent.ID] = target
	p.order = append(p.order, ent.ID)
	return target
}

// record remembers one path the batch writes, in first-write order.
func (t *targetWrites) record(path string) {
	if _, ok := t.touched[path]; ok {
		return
	}
	t.touched[path] = struct{}{}
	t.paths = append(t.paths, path)
}

// read resolves one path on the working copy, falling back to the entity so
// template defaults are visible to an add.
func (t *targetWrites) read(path string) any {
	if value, ok := readPath(t.work, path); ok {
		return value
	}
	if value, err := t.entity.Get(path); err == nil {
		return value
	}
	return nil
}

// commit creates the queued materials first and then writes every touched path.
// A refused write rolls back the writes and the creations already performed; an
// incomplete rollback is reported instead of being hidden.
func (p *applyPlan) commit() error {
	created := make([]string, 0, len(p.creates))
	for _, create := range p.creates {
		if _, err := p.store.Create(create.id, create.typeName, create.data, nil); err != nil {
			p.deleteCreated(created)
			return fmt.Errorf("create material %q: %w", create.id, err)
		}
		created = append(created, create.id)
	}
	applied := make([]appliedWrite, 0, p.writeCount())
	for _, id := range p.order {
		target := p.targets[id]
		for _, path := range target.paths {
			if err := p.write(target, path, &applied, created); err != nil {
				return err
			}
		}
	}
	return nil
}

// write performs one entity write and records it for rollback.
func (p *applyPlan) write(target *targetWrites, path string, applied *[]appliedWrite, created []string) error {
	value, ok := readPath(target.work, path)
	if !ok {
		if rollbackErr := p.rollback(*applied, created); rollbackErr != nil {
			return fmt.Errorf("internal: working copy lost %q; rollback incomplete: %v", path, rollbackErr)
		}
		return fmt.Errorf("internal: working copy lost %q", path)
	}
	prior, hasPrior := readPath(target.before, path)
	if !hasPrior {
		if current, err := target.entity.Get(path); err == nil {
			prior, hasPrior = current, true
		}
	}
	if err := target.entity.Set(path, value); err != nil {
		if rollbackErr := p.rollback(*applied, created); rollbackErr != nil {
			return fmt.Errorf("%s: write refused (%w); rollback incomplete: %v", path, err, rollbackErr)
		}
		return fmt.Errorf("%s: write refused: %w", path, err)
	}
	*applied = append(*applied, appliedWrite{target: target, path: path, prior: prior, hasPrior: hasPrior})
	return nil
}

// rollback restores every performed write in reverse order and deletes the
// materials the batch created. A path with no pre-batch value cannot be removed
// by entity.Set, so it is reported rather than silently ignored.
func (p *applyPlan) rollback(applied []appliedWrite, created []string) error {
	problems := make([]string, 0, len(applied))
	for index := len(applied) - 1; index >= 0; index-- {
		write := applied[index]
		if !write.hasPrior {
			problems = append(problems, fmt.Sprintf("%s.%s has no pre-batch value", write.target.entity.ID, write.path))
			continue
		}
		if err := write.target.entity.Set(write.path, write.prior); err != nil {
			problems = append(problems, fmt.Sprintf("%s.%s restore: %v", write.target.entity.ID, write.path, err))
		}
	}
	p.deleteCreated(created)
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// deleteCreated removes the materials a failed batch created.
func (p *applyPlan) deleteCreated(ids []string) {
	for _, id := range ids {
		p.store.Delete(id)
	}
}

// writeCount counts the entity writes commit will perform.
func (p *applyPlan) writeCount() int {
	total := 0
	for _, id := range p.order {
		total += len(p.targets[id].paths)
	}
	return total
}
