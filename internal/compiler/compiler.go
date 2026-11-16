package compiler

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/neko"
)

// Result is the output of compiling one .neko script.
type Result struct {
	Templates       map[string]map[string]any
	TemplateParents map[string][]string
	Rules           []engine.Rule
	Funcs           []neko.FuncDef
	Events          []neko.EventDef
	Errors          []error
	Warnings        []neko.Warning
}

// OK reports whether compilation produced no errors.
func (r *Result) OK() bool { return len(r.Errors) == 0 }

func shortHash(text string) string {
	sum := md5.Sum([]byte(text))
	return hex.EncodeToString(sum[:])[:8]
}

// ExpandAnnotations rewrites "体力[int,@事件]: 75" keys into plain fields and
// returns the annotation table keyed by full field path.
func ExpandAnnotations(data map[string]any) (map[string]any, map[string]any) {
	result := map[string]any{}
	annotations := map[string]any{}
	for key, value := range data {
		if name, schema, def, ok := entity.ParseAnnotatedField(key); ok {
			chosen := value
			if def != nil {
				chosen = def
			}
			result[name] = chosen
			annotations[name] = annotationMap(schema)
			continue
		}
		child, childOK := value.(map[string]any)
		if !childOK {
			result[key] = value
			continue
		}
		sub, subAnnotations := ExpandAnnotations(child)
		delete(sub, "__annotations__")
		result[key] = sub
		for name, meta := range subAnnotations {
			annotations[key+"."+name] = meta
		}
	}
	if len(annotations) > 0 {
		result["__annotations__"] = annotations
	}
	return result, annotations
}

// annotationMap renders one field annotation for the template __annotations__
// table. min/max are emitted as plain values because every reader
// (entity.toFloat and engine.toFloatValue) accepts JSON-shaped numbers and
// does not dereference *float64.
func annotationMap(schema *entity.FieldSchema) map[string]any {
	var minValue, maxValue any
	if schema.Min != nil {
		minValue = *schema.Min
	}
	if schema.Max != nil {
		maxValue = *schema.Max
	}
	return map[string]any{
		"type": schema.Type, "event": schema.Event, "enum": schema.Enum,
		"min": minValue, "max": maxValue,
		"readonly": schema.Readonly, "nullable": schema.Nullable,
	}
}

// CompileScript compiles every definition in the AST.
func CompileScript(ast *neko.ScriptAST) *Result {
	result := &Result{
		Templates:       map[string]map[string]any{},
		TemplateParents: map[string][]string{},
		Rules:           []engine.Rule{},
		Funcs:           []neko.FuncDef{}, Events: []neko.EventDef{},
		Errors: []error{}, Warnings: []neko.Warning{},
	}
	for _, def := range ast.DataDefs {
		if !def.IsEntity {
			continue
		}
		// Duplicate @Data types inside one script used to overwrite each other
		// silently. Report the conflict so the author fixes the source instead
		// of losing fields at compile time.
		if _, exists := result.Templates[def.TypeName]; exists {
			result.Errors = append(result.Errors, &neko.CompileError{
				Message: fmt.Sprintf("模板 '%s' 在同一脚本内重复声明", def.TypeName),
				Pos:     def.Pos, Block: "模板",
			})
			continue
		}
		expanded, _ := ExpandAnnotations(def.Data)
		result.Templates[def.TypeName] = expanded
		if len(def.Parents) > 0 {
			if previous, ok := result.TemplateParents[def.TypeName]; ok && !entity.SameParents(previous, def.Parents) {
				result.Errors = append(result.Errors, &neko.CompileError{
					Message: fmt.Sprintf("模板 '%s' 在同一脚本内重复声明且父列表不一致", def.TypeName),
					Pos:     def.Pos, Block: "模板组合",
				})
				continue
			}
			result.TemplateParents[def.TypeName] = append([]string(nil), def.Parents...)
		}
	}
	result.Funcs = append(result.Funcs, ast.FuncDefs...)
	for _, event := range ast.EventDefs {
		if len(event.Actions) > 0 {
			result.Warnings = append(result.Warnings, neko.Warning{
				Message: fmt.Sprintf("事件 '%s' 的动作行不会执行，请改用 @函数 处理器", event.Name),
				Pos:     event.Pos,
			})
		}
	}
	result.Events = append(result.Events, ast.EventDefs...)
	for _, def := range ast.RuleDefs {
		rule, err := CompileRule(def)
		if err != nil {
			result.Errors = append(result.Errors, &neko.CompileError{
				Message: err.Error(), Pos: def.Pos, Block: "规则",
			})
			continue
		}
		result.Rules = append(result.Rules, *rule)
	}
	result.Warnings = append(result.Warnings, ast.Warnings...)
	return result
}
