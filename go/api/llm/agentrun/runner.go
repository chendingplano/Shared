// Package agentrun provides the Runner abstraction ChenWeb's agent platform
// uses to execute coding-agent CLIs (Claude Code, Codex, OpenClaw, OpenCode)
// inside sandboxed containers.
//
// Each Runner is responsible for one agent CLI. The worker pool in
// ChenWeb/server/api/agentplatformhandler picks up ap_task_run rows and
// drives them through Prepare → Run → Collect, persisting events and
// artifacts as they arrive.
//
// Nothing here imports ChenWeb packages; the dependency direction is
// strictly one-way (ChenWeb → shared) so other projects can reuse the same
// primitives.
package agentrun

import (
	"context"
	"time"
)

// TaskSpec carries everything a Runner needs to execute one task.
// It is assembled by the worker from ap_task_run, ap_issue, and ap_agent rows.
type TaskSpec struct {
	RunID        string // ap_task_run.id
	IssueID      string
	IssueNumber  int
	IssueTitle   string
	IssueDesc    string
	AgentID      string
	AgentName    string
	RuntimeKind  string // "claude_code" | "codex" | "openclaw" | "opencode" | "dryrun"
	Model        string // optional override ("" = runner default)
	Instructions string // per-agent system prompt

	// WorkdirPath is the absolute host path the Runner prepares (and the
	// sandbox mounts at /workspace). The worker assigns this — typically
	// /srv/agentplatform/workdirs/<run_id>/.
	WorkdirPath string

	// EnvSecrets are injected into the container as --env key=value.
	// The worker populates this from server env (e.g. CLAUDE_API_KEY) at
	// dispatch time; secrets never pass through the DB.
	EnvSecrets map[string]string

	// NetworkPolicy is "none" (default; offline agent) or "bridge" (agent
	// needs internet, e.g. to call an LLM provider API).
	NetworkPolicy string

	// Timeout caps Run()'s duration. Zero means "no cap" (relies on ctx).
	Timeout time.Duration
}

// WorkDir is the prepared working directory returned by Runner.Prepare.
// The worker is responsible for creating/cleaning the host path;
// the sandbox layer mounts it into the container.
type WorkDir struct {
	HostPath string
}

// Artifact describes a file left in the workdir by a run.
// The worker persists these as ap_artifact rows after Collect returns.
type Artifact struct {
	Path      string // absolute host path
	Kind      string // "diff" | "log" | "file" | "other"
	SizeBytes int64
}

// Runner is the interface implemented by each agent-CLI wrapper.
// Implementations MUST be stateless (one Runner instance may process many
// tasks concurrently); put per-task state in TaskSpec / WorkDir.
type Runner interface {
	// Kind returns a stable identifier matching ap_agent.runtime_kind.
	Kind() string

	// Prepare creates the workdir, writes any scaffolding files (e.g.
	// ISSUE.md with the issue body), and initializes any runtime state.
	// It must be idempotent — a lease-reclaimed task may re-Prepare.
	Prepare(ctx context.Context, spec TaskSpec) (WorkDir, error)

	// Run executes the agent CLI inside a sandbox, streaming events to out.
	// It MUST respect ctx cancellation: on ctx.Err, kill the subprocess
	// and return the error. Closing out is the caller's responsibility.
	Run(ctx context.Context, wd WorkDir, spec TaskSpec, out chan<- Event) error

	// Collect inspects the workdir after Run returns and returns any
	// artifacts worth persisting (diffs, logs, generated files).
	Collect(ctx context.Context, wd WorkDir) ([]Artifact, error)
}
