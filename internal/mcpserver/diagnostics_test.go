package mcpserver

import (
	"testing"

	"github.com/ruicat/astrbox/internal/diagnostics"
	"github.com/ruicat/astrbox/internal/entity"
)

func TestAttachDiagnosticsModes(t *testing.T) {
	server := &Server{diagnostics: diagnostics.New(16)}
	ent := &entity.Entity{ID: "猫猫", Type: "NPC:猫猫"}
	server.diagnostics.Add(diagnostics.Record{
		Code: "tick.rule.error", Severity: diagnostics.SEVERITY_ERROR,
		Entity: "猫猫", Message: "boom",
	})

	payload := map[string]any{}
	server.attachDiagnostics(payload, ent, "")
	if _, ok := payload["diagnostics"]; !ok {
		t.Fatal("default summary missing")
	}

	payload = map[string]any{}
	server.attachDiagnostics(payload, ent, "none")
	if _, ok := payload["diagnostics"]; ok {
		t.Fatal("none mode attached diagnostics")
	}

	payload = map[string]any{}
	server.attachDiagnostics(payload, ent, "full")
	scope, ok := payload["diagnostics"].(diagnostics.Scope)
	if !ok {
		t.Fatalf("full mode payload = %T", payload["diagnostics"])
	}
	if scope.Summary.Error != 1 || len(scope.Items) != 1 {
		t.Fatalf("scope = %+v", scope)
	}
}
