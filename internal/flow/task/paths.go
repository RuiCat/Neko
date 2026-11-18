// paths.go declares the closed set of entity fields this package writes and the
// stage vocabulary of the task contract. C4 makes the two task state rows
// single-writer, so every write below is spelled here once and nowhere else.
package task

// Task state rows owned by this package (C4). The task rules in
// scripts/rules/task/taskRules.neko own the stage row (tkStart/tkFinish) and the
// completion mark (tkReward); they read these two rows to drive the stage
// machine, so one completion is written as:
//
//	任务.当前 = completion.FlowID   (activation edge, only when it changes)
//	任务.进度 = 100                  (completion edge, every advance)
//
// The stage row itself stays with the rules: this package only reads it, which is
// what keeps the stage machine free of a second writer.
const (
	// PATH_TASK_CURRENT is the active task ID row.
	PATH_TASK_CURRENT = "任务.当前"
	// PATH_TASK_PROGRESS is the 0-100 progress row.
	PATH_TASK_PROGRESS = "任务.进度"
	// PATH_TASK_STAGE is the rule-owned stage row; read-only here.
	PATH_TASK_STAGE = "任务.阶段"
	// PATH_GROWTH_EXPERIENCE is the growth kernel experience row.
	PATH_GROWTH_EXPERIENCE = "成长.经验"
	// PATH_RELATION_AFFINITY is the C4 affinity row.
	PATH_RELATION_AFFINITY = "关系.数值.好感度"
)

// Stage values of the task contract. The extractor maps the corpus 已激活 to
// STAGE_INACTIVE (C4), so these three are the only stages Advance accepts.
const (
	// STAGE_INACTIVE is a task that has not started.
	STAGE_INACTIVE = "未激活"
	// STAGE_ACTIVE is a task in progress.
	STAGE_ACTIVE = "进行中"
	// STAGE_COMPLETED is a finished task; a second completion must not pay again.
	STAGE_COMPLETED = "完成"
)

// PROGRESS_COMPLETE is the progress value that lets the tkFinish rule move the
// task from 进行中 to 完成.
const PROGRESS_COMPLETE = 100
