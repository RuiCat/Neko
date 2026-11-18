// validate.go checks the K1 invariants m1 promises to m2-m6: schema version,
// record count, reopenable file:line anchors, effect kinds and the five-file
// snapshot hashes. Generation runs these checks before returning artifacts.
package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ValidateArtifacts cross-checks the three artifacts and every stated invariant.
func ValidateArtifacts(artifacts Artifacts) error {
	if err := ValidateFlowSpecFile(artifacts.Flows); err != nil {
		return err
	}
	if err := ValidateEffectMap(artifacts.Effects); err != nil {
		return err
	}
	if err := ValidateMigrationReport(artifacts.Report); err != nil {
		return err
	}
	if err := ValidateAnchors(artifacts.Report); err != nil {
		return err
	}
	if err := ValidateSnapshotHashes(artifacts.Report); err != nil {
		return err
	}
	classified := 0
	for _, flow := range artifacts.Flows.Flows {
		classified += len(flow.Effects)
	}
	if classified != len(artifacts.Effects.Effects) {
		return fmt.Errorf("effect-map has %d effects, flow specs carry %d", len(artifacts.Effects.Effects), classified)
	}
	taskRecords := 0
	flowRecords := 0
	r18Records := 0
	for _, record := range artifacts.Report.Records {
		switch record.Kind {
		case "taskTemplate":
			taskRecords++
		case "flow":
			flowRecords++
		case "r18Flow":
			r18Records++
		default:
			return fmt.Errorf("record %s:%d has unknown kind %q", record.File, record.Line, record.Kind)
		}
	}
	if taskRecords != len(artifacts.Flows.TaskTemplates) {
		return fmt.Errorf("records carry %d taskTemplates, flow-spec has %d", taskRecords, len(artifacts.Flows.TaskTemplates))
	}
	if flowRecords+r18Records != len(artifacts.Flows.Flows) {
		return fmt.Errorf("records carry %d flow+r18Flow, flow-spec has %d flows", flowRecords+r18Records, len(artifacts.Flows.Flows))
	}
	return nil
}

// ValidateFlowSpecFile checks flow-spec.json structure and effect literals.
func ValidateFlowSpecFile(file FlowSpecFile) error {
	if file.SchemaVersion != SCHEMA_VERSION {
		return fmt.Errorf("flow-spec schemaVersion = %d, want %d", file.SchemaVersion, SCHEMA_VERSION)
	}
	seen := make(map[string]struct{}, len(file.Flows))
	for i, flow := range file.Flows {
		where := fmt.Sprintf("flows[%d] %q", i, flow.ID)
		if strings.TrimSpace(flow.ID) == "" {
			return fmt.Errorf("%s: empty id", where)
		}
		if flow.Source.File == "" || flow.Source.Line <= 0 {
			return fmt.Errorf("%s: missing source file:line", where)
		}
		if flow.Cost < 0 {
			return fmt.Errorf("%s: negative cost %d", where, flow.Cost)
		}
		if _, duplicate := seen[flow.ID]; duplicate {
			return fmt.Errorf("%s: duplicate flow id", where)
		}
		seen[flow.ID] = struct{}{}
		for j, effect := range flow.Effects {
			if err := validateEffect(effect); err != nil {
				return fmt.Errorf("%s effect[%d]: %w", where, j, err)
			}
		}
	}
	seenTemplates := make(map[string]struct{}, len(file.TaskTemplates))
	for i, template := range file.TaskTemplates {
		where := fmt.Sprintf("taskTemplates[%d] %q", i, template.ID)
		if strings.TrimSpace(template.ID) == "" {
			return fmt.Errorf("%s: empty id", where)
		}
		if template.Source.File == "" || template.Source.Line <= 0 {
			return fmt.Errorf("%s: missing source file:line", where)
		}
		if len(template.Stages) == 0 {
			return fmt.Errorf("%s: empty stages", where)
		}
		for _, stage := range template.Stages {
			if strings.TrimSpace(stage) == "" {
				return fmt.Errorf("%s: blank stage", where)
			}
			if stage == "已激活" {
				return fmt.Errorf("%s: stage 已激活 must be mapped to 未激活", where)
			}
		}
		if strings.TrimSpace(template.Condition.Type) == "" {
			return fmt.Errorf("%s: empty condition.type", where)
		}
		if template.Reward.Experience < 0 || template.Reward.Affinity < 0 {
			return fmt.Errorf("%s: negative reward", where)
		}
		identity := fmt.Sprintf("%s\x00%d\x00%s", template.Source.File, template.Source.Line, template.ID)
		if _, duplicate := seenTemplates[identity]; duplicate {
			return fmt.Errorf("%s: duplicate anchor", where)
		}
		seenTemplates[identity] = struct{}{}
	}
	return nil
}

// ValidateEffectMap checks the C2 artifact: every effect has a closed kind and
// the unclassified count matches its detail list.
func ValidateEffectMap(effectMap EffectMap) error {
	if effectMap.SchemaVersion != SCHEMA_VERSION {
		return fmt.Errorf("effect-map schemaVersion = %d, want %d", effectMap.SchemaVersion, SCHEMA_VERSION)
	}
	for i, effect := range effectMap.Effects {
		if err := validateEffect(effect); err != nil {
			return fmt.Errorf("effects[%d]: %w", i, err)
		}
	}
	if effectMap.Unclassified != len(effectMap.UnclassifiedItems) {
		return fmt.Errorf("unclassified = %d, detail items = %d", effectMap.Unclassified, len(effectMap.UnclassifiedItems))
	}
	return nil
}

// ValidateMigrationReport checks counts and record anchors' shape. Reopening
// the anchors is ValidateAnchors; hash equality is ValidateSnapshotHashes.
func ValidateMigrationReport(report MigrationReport) error {
	if report.SchemaVersion != SCHEMA_VERSION {
		return fmt.Errorf("migration-report schemaVersion = %d, want %d", report.SchemaVersion, SCHEMA_VERSION)
	}
	if report.SnapshotSha256 == "" || report.SnapshotSha256Compat == "" {
		return fmt.Errorf("migration-report is missing snapshot hashes")
	}
	if len(report.SnapshotFiles) == 0 {
		return fmt.Errorf("migration-report is missing per-file hashes")
	}
	if report.Count != len(report.Records) {
		return fmt.Errorf("count = %d, records = %d", report.Count, len(report.Records))
	}
	if report.ContainerCount != len(report.Containers) {
		return fmt.Errorf("containerCount = %d, containers = %d", report.ContainerCount, len(report.Containers))
	}
	if report.SkippedCount != len(report.Skipped) {
		return fmt.Errorf("skippedCount = %d, skipped = %d", report.SkippedCount, len(report.Skipped))
	}
	if report.ErrorCount != len(report.Errors) {
		return fmt.Errorf("errorCount = %d, errors = %d", report.ErrorCount, len(report.Errors))
	}
	seen := make(map[string]struct{}, len(report.Records))
	for i, record := range report.Records {
		if record.File == "" || record.Line <= 0 {
			return fmt.Errorf("records[%d]: missing file:line", i)
		}
		if strings.TrimSpace(record.Key) == "" {
			return fmt.Errorf("records[%d]: empty key", i)
		}
		if record.Kind == "" {
			return fmt.Errorf("records[%d]: empty kind", i)
		}
		identity := fmt.Sprintf("%s\x00%d\x00%s", record.File, record.Line, record.Key)
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("records[%d]: duplicate anchor %s:%d %q", i, record.File, record.Line, record.Key)
		}
		seen[identity] = struct{}{}
	}
	return nil
}

// ValidateAnchors reopens every record file and checks that the recorded line
// contains the recorded key, exactly as the C1 judgment script does.
func ValidateAnchors(report MigrationReport) error {
	cache := make(map[string][]string)
	for i, record := range report.Records {
		lines, ok := cache[record.File]
		if !ok {
			data, err := os.ReadFile(record.File)
			if err != nil {
				return fmt.Errorf("records[%d] %s:%d: %w", i, record.File, record.Line, err)
			}
			lines = strings.Split(string(data), "\n")
			cache[record.File] = lines
		}
		if record.Line > len(lines) {
			return fmt.Errorf("records[%d] %s:%d: line out of range (%d lines)", i, record.File, record.Line, len(lines))
		}
		if !strings.Contains(lines[record.Line-1], record.Key) {
			return fmt.Errorf("records[%d] %s:%d: line does not contain key %q", i, record.File, record.Line, record.Key)
		}
	}
	return nil
}

// ValidateSnapshotHashes recomputes the per-file hashes, the concatenated
// digest and the sha256sum manifest, and compares them with the report.
func ValidateSnapshotHashes(report MigrationReport) error {
	if len(report.SnapshotFiles) == 0 {
		return fmt.Errorf("snapshotFiles is empty")
	}
	combined := sha256.New()
	var manifest strings.Builder
	for _, fileHash := range report.SnapshotFiles {
		data, err := os.ReadFile(fileHash.File)
		if err != nil {
			return fmt.Errorf("snapshot %s: %w", fileHash.File, err)
		}
		sum := sha256.Sum256(data)
		sumHex := hex.EncodeToString(sum[:])
		if sumHex != fileHash.Sha256 {
			return fmt.Errorf("snapshot %s: sha256 %s, report says %s", fileHash.File, sumHex, fileHash.Sha256)
		}
		combined.Write(data)
		manifest.WriteString(sumHex)
		manifest.WriteString("  ")
		manifest.WriteString(fileHash.File)
		manifest.WriteByte('\n')
	}
	if got := hex.EncodeToString(combined.Sum(nil)); got != report.SnapshotSha256 {
		return fmt.Errorf("snapshotSha256 = %s, recomputed %s", report.SnapshotSha256, got)
	}
	if manifest.String() != report.SnapshotSha256Compat {
		return fmt.Errorf("snapshot_sha256 does not match the sha256sum manifest")
	}
	return nil
}

func validateEffect(effect EffectSpec) error {
	if strings.TrimSpace(effect.Path) == "" {
		return fmt.Errorf("empty path")
	}
	if effect.Source.File == "" || effect.Source.Line <= 0 {
		return fmt.Errorf("%s: missing source file:line", effect.Path)
	}
	if strings.TrimSpace(effect.TargetReason) == "" {
		return fmt.Errorf("%s: missing targetReason", effect.Path)
	}
	if effect.Target == nil {
		// A null target is legal only when the reason explains why.
		if strings.TrimSpace(effect.TargetReason) == "" {
			return fmt.Errorf("%s: null target without reason", effect.Path)
		}
	} else {
		target := strings.TrimSpace(*effect.Target)
		if target == "" {
			return fmt.Errorf("%s: blank target", effect.Path)
		}
		if isForbiddenTarget(target) {
			return fmt.Errorf("%s: target %q is a C3-forbidden projection path", effect.Path, target)
		}
		if !isAllowedTarget(target) {
			return fmt.Errorf("%s: target %q is not an allowed sourcePaths field", effect.Path, target)
		}
	}
	switch effect.Kind {
	case EFFECT_KIND_SET, EFFECT_KIND_ADD:
		if !isNumeric(effect.Value) {
			return fmt.Errorf("%s: kind %s needs a number, got %T", effect.Path, effect.Kind, effect.Value)
		}
	case EFFECT_KIND_TEXT:
		if _, ok := effect.Value.(string); !ok {
			return fmt.Errorf("%s: kind text needs a string, got %T", effect.Path, effect.Value)
		}
	case EFFECT_KIND_BOOL:
		if _, ok := effect.Value.(bool); !ok {
			return fmt.Errorf("%s: kind bool needs a bool, got %T", effect.Path, effect.Value)
		}
	default:
		return fmt.Errorf("%s: unknown kind %q", effect.Path, effect.Kind)
	}
	return nil
}

func isNumeric(value any) bool {
	switch number := value.(type) {
	case json.Number:
		_, err := strconv.ParseFloat(number.String(), 64)
		return err == nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}
