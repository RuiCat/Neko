// Package task consumes K4 completion events and advances the task state machine
// 未激活 → 进行中 → 完成. It is the single writer of the two task state rows C4
// reserves for it (任务.当前 / 任务.进度), pays each template reward exactly once,
// and extracts the C7 R18 flow artifact from the read-only event digest corpus.
package task
