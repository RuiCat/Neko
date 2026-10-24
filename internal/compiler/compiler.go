package compiler

import (
	"crypto/md5"
	"encoding/hex"

	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/neko"
)

// Result is the output of compiling one .neko script.
type Result struct {
	Templates map[string]map[string]any
	Rules     []engine.Rule
	Funcs     []neko.FuncDef
	Events    []neko.EventDef
	Errors    []error
	Warnings  []neko.Warning
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

func annotationMap(schema *entity.FieldSchema) map[string]any {
	return map[string]any{
		"type": schema.Type, "event": schema.Event, "enum": schema.Enum,
		"min": schema.Min, "max": schema.Max,
		"readonly": schema.Readonly, "nullable": schema.Nullable,
	}
}

// CompileScript compiles every definition in the AST.
func CompileScript(ast *neko.ScriptAST) *Result {
	result := &Result{
		Templates: map[string]map[string]any{}, Rules: []engine.Rule{},
		Funcs: []neko.FuncDef{}, Events: []neko.EventDef{},
		Errors: []error{}, Warnings: []neko.Warning{},
	}
	for _, def := range ast.DataDefs {
		if !def.IsEntity {
			continue
		}
		expanded, _ := ExpandAnnotations(def.Data)
		result.Templates[def.TypeName] = expanded
	}
	result.Funcs = append(result.Funcs, ast.FuncDefs...)
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
