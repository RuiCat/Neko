// Status reconciliation from DSH session/list so a missing bridge cannot leave
// a context stuck in "running".
package driver

import (
	"context"
)

// RefreshStatus reads session/list and rewrites each known context's status and
// last turn from the session's live row. Sessions missing from the list keep
// their stored values; a not-yet-created session stays whatever it was.
func (d *Driver) RefreshStatus(ctx context.Context) error {
	summaries, err := d.client.List(ctx)
	if err != nil {
		return err
	}
	bySession := make(map[string]SessionSummary, len(summaries))
	for _, summary := range summaries {
		bySession[summary.SessionID] = summary
	}
	for _, record := range d.registry.List() {
		summary, ok := bySession[record.SessionID]
		if !ok || record.SessionID == "" {
			continue
		}
		target := STATUS_IDLE
		if summary.Running {
			target = STATUS_RUNNING
		}
		turns := 0
		if summary.Projections != nil {
			turns = summary.Projections.Values.SessionStats.Turns
		}
		if record.Status == target && turns <= record.LastTurn {
			continue
		}
		if _, err := d.registry.Update(record.ContextID, func(record *Record) {
			record.Status = target
			if turns > record.LastTurn {
				record.LastTurn = turns
			}
		}); err != nil {
			return err
		}
	}
	return nil
}
