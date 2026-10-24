package entity

import (
	"testing"

	"github.com/ruicat/astrbox/internal/config"
)

func TestConfiguredClassificationField(t *testing.T) {
	if err := config.Set(config.Config{
		StateGroup: "状态", StateDefinitionField: "状态定义", DefaultStateSubgroup: "身体",
		GlobalEntities: []string{"场景"}, DefaultScene: "场景",
		SceneEventField: "事件", ClassificationField: "标签",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.Reset)

	store := NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("猫猫", "NPC:猫猫", map[string]any{"标签": []any{"成年"}}, nil); err != nil {
		t.Fatal(err)
	}
	entities, err := store.Query("成年[*]")
	if err != nil {
		t.Fatal(err)
	}
	if len(entities) != 1 || entities[0].ID != "猫猫" {
		t.Fatalf("query = %+v", entities)
	}
}
