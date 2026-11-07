// repair.go rolls back the newest live repair version and removes the engine
// rules that version installed; shared by POST /repairs/rollback and the
// repair_rollback tool.
package ops

import (
	"github.com/RuiCat/Neko/internal/repair"
)

// RollbackRepair rolls back the newest live repair version. The result mirrors
// the POST /repairs/rollback response body.
func RollbackRepair(deps Deps) (map[string]any, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}
	version, err := deps.repairStore().Rollback()
	if err != nil {
		return nil, Internal("%s", err)
	}
	for _, ruleID := range version.Applied {
		deps.Machine.RemoveRule(ruleID)
		_, _ = repair.RemoveRule(repair.RulesPath(deps.DataDir), ruleID)
	}
	return map[string]any{"ok": true, "rolled_back": version}, nil
}
