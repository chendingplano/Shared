package agentrun

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultClaudeImage = "chenweb/agentrun-claude:v1"
	claudeImageEnvVar  = "AGENT_PLATFORM_CLAUDE_IMAGE"
)

// ClaudeCodeRunner runs Anthropic's Claude Code CLI inside the
// chenweb/agentrun-claude container. Prepare writes ISSUE.md, Run
// launches the container (which reads ISSUE.md, invokes `claude --print`,
// and edits files in /workspace), Collect walks the workdir for artifacts.
//
// Dockerfile: ChenWeb/docker/agentrun-claude/Dockerfile.
type ClaudeCodeRunner struct{}

func (ClaudeCodeRunner) Kind() string { return "claude_code" }

// Version implements the optional Versioned interface consumed by the worker
// when it stamps ap_task_run.runner_version — gives the image tag used.
func (ClaudeCodeRunner) Version() string {
	if v := os.Getenv(claudeImageEnvVar); v != "" {
		return v
	}
	return defaultClaudeImage
}

func (ClaudeCodeRunner) Prepare(_ context.Context, spec TaskSpec) (WorkDir, error) {
	if err := os.MkdirAll(spec.WorkdirPath, 0o755); err != nil {
		return WorkDir{}, fmt.Errorf("mkdir workdir: %w", err)
	}
	content := formatIssueMarkdown(spec)
	path := filepath.Join(spec.WorkdirPath, "ISSUE.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return WorkDir{}, fmt.Errorf("write ISSUE.md: %w", err)
	}
	return WorkDir{HostPath: spec.WorkdirPath}, nil
}

func (ClaudeCodeRunner) Run(ctx context.Context, wd WorkDir, spec TaskSpec, out chan<- Event) error {
	// Refuse up-front if no provider key is wired — the container would
	// fail with a generic error otherwise.
	if !hasAnyKey(spec.EnvSecrets, "ANTHROPIC_API_KEY", "CLAUDE_API_KEY") {
		return fmt.Errorf("claude_code runner requires ANTHROPIC_API_KEY (or CLAUDE_API_KEY) in the server env")
	}

	image := defaultClaudeImage
	if v := os.Getenv(claudeImageEnvVar); v != "" {
		image = v
	}

	network := spec.NetworkPolicy
	if network == "" || network == "none" {
		// Claude Code must reach api.anthropic.com.
		network = "bridge"
	}

	sb := DockerSandbox{
		Image:       image,
		WorkdirHost: wd.HostPath,
		Network:     network,
		Env:         spec.EnvSecrets,
		MemoryMB:    2048,
		CPUs:        "2",
	}
	return sb.Run(ctx, out)
}

// Collect walks the workdir and returns every non-hidden file as an Artifact.
// Kind is inferred from the extension (diff/patch → "diff", .log → "log",
// anything else → "file"). File contents are NOT read here; only metadata.
func (ClaudeCodeRunner) Collect(_ context.Context, wd WorkDir) ([]Artifact, error) {
	var out []Artifact
	err := filepath.WalkDir(wd.HostPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip; do not fail the whole collect
		}
		if d.IsDir() {
			// Skip dotdirs (.git, etc.) but keep the workdir root itself.
			if path != wd.HostPath && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, Artifact{
			Path:      path,
			Kind:      artifactKind(d.Name()),
			SizeBytes: info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk workdir: %w", err)
	}
	return out, nil
}

func artifactKind(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".diff"), strings.HasSuffix(lower, ".patch"):
		return "diff"
	case strings.HasSuffix(lower, ".log"):
		return "log"
	default:
		return "file"
	}
}

func hasAnyKey(m map[string]string, keys ...string) bool {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != "" {
			return true
		}
	}
	return false
}
