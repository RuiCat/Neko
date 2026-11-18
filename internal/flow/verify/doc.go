// Package verify is the P4 acceptance harness. It replays the three artifact
// flows through condition, step and settle, advances one task completion twice,
// replays the R18 steps through an independent digest and revalidates the
// migration report. It owns no business logic and never rewrites an artifact.
package verify
