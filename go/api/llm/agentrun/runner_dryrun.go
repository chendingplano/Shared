package agentrun

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DryRunRunner is a no-op runner used to exercise the task pipeline before
// the real agent runners land in M1c. It writes an ISSUE.md scaffold,
// emits a handful of stdout lines, and reports success. Intentionally does
// not launch a container — it runs fully in-process.
type DryRunRunner struct{}

func (DryRunRunner) Kind() string { return "dryrun" }

func (DryRunRunner) Prepare(_ context.Context, spec TaskSpec) (WorkDir, error) {
	if err := os.MkdirAll(spec.WorkdirPath, 0o755); err != nil {
		return WorkDir{}, fmt.Errorf("mkdir workdir: %w", err)
	}
	content := formatIssueMarkdown(spec)
	issuePath := filepath.Join(spec.WorkdirPath, "ISSUE.md")
	if err := os.WriteFile(issuePath, []byte(content), 0o644); err != nil {
		return WorkDir{}, fmt.Errorf("write ISSUE.md: %w", err)
	}
	return WorkDir{HostPath: spec.WorkdirPath}, nil
}

func (DryRunRunner) Run(ctx context.Context, _ WorkDir, spec TaskSpec, out chan<- Event) error {
	lines := []string{
		"dryrun: starting",
		fmt.Sprintf("dryrun: issue=%q agent=%q kind=%s", spec.IssueTitle, spec.AgentName, spec.RuntimeKind),
		"dryrun: (no real agent executed — set AGENT_PLATFORM_FORCE_DRYRUN=false and install M1c to run for real)",
		"dryrun: done",
	}
	for _, l := range lines {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- Event{Kind: EventStdout, Payload: l, At: time.Now()}:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil
}

func (DryRunRunner) Collect(_ context.Context, wd WorkDir) ([]Artifact, error) {
	path := filepath.Join(wd.HostPath, "ISSUE.md")
	fi, err := os.Stat(path)
	if err != nil {
		return nil, nil
	}
	return []Artifact{{Path: path, Kind: "file", SizeBytes: fi.Size()}}, nil
}
