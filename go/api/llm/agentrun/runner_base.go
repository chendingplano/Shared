package agentrun

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// This file holds helpers shared by every Docker-sandboxed runner
// (Claude Code, Codex, OpenClaw, OpenCode). Each concrete runner is a
// thin shell around these; the idea is that adding a new CLI kind means
// declaring a DockerRunnerConfig, not rewriting Prepare/Run/Collect.

// -----------------------------------------------------------------------------
// DockerRunnerConfig — a declarative description of one CLI runtime.
// -----------------------------------------------------------------------------

// DockerRunnerConfig parameterizes a Docker-sandboxed CLI runner.
// Instances are value types; concrete runner structs embed one and
// forward Run through runSandboxed.
type DockerRunnerConfig struct {
	// Kind is the ap_agent.runtime_kind string this runner represents.
	Kind string

	// DefaultImage is the tag used when ImageEnvVar is unset.
	DefaultImage string

	// ImageEnvVar lets operators override the image tag at runtime
	// without rebuilding (e.g. "AGENT_PLATFORM_CODEX_IMAGE").
	ImageEnvVar string

	// RequiredKeys is the set of provider API-key env names the runner
	// refuses to start without. Any one being present is sufficient.
	// Example: {"ANTHROPIC_API_KEY", "CLAUDE_API_KEY"}.
	RequiredKeys []string

	// MemoryMB and CPUs are the container resource caps. 0 / "" means
	// "use the shared default" (2048MB / 2 CPUs).
	MemoryMB int
	CPUs     string
}

// ResolveImage returns the image tag the runner should launch, honoring
// the per-kind env override when present.
func (cfg DockerRunnerConfig) ResolveImage() string {
	if cfg.ImageEnvVar != "" {
		if v := os.Getenv(cfg.ImageEnvVar); v != "" {
			return v
		}
	}
	return cfg.DefaultImage
}

// runSandboxed is the common Run body. It validates the required keys,
// picks network policy, and hands off to DockerSandbox.
func (cfg DockerRunnerConfig) runSandboxed(ctx context.Context, wd WorkDir, spec TaskSpec, out chan<- Event) error {
	if len(cfg.RequiredKeys) > 0 && !hasAnyKey(spec.EnvSecrets, cfg.RequiredKeys...) {
		return fmt.Errorf("%s runner requires one of %v in the server env", cfg.Kind, cfg.RequiredKeys)
	}
	network := spec.NetworkPolicy
	if network == "" || network == "none" {
		// Every real agent in M3 calls an external LLM API.
		network = "bridge"
	}
	mem := cfg.MemoryMB
	if mem == 0 {
		mem = 2048
	}
	cpus := cfg.CPUs
	if cpus == "" {
		cpus = "2"
	}
	sb := DockerSandbox{
		Image:       cfg.ResolveImage(),
		WorkdirHost: wd.HostPath,
		Network:     network,
		Env:         spec.EnvSecrets,
		MemoryMB:    mem,
		CPUs:        cpus,
	}
	return sb.Run(ctx, out)
}

// -----------------------------------------------------------------------------
// Shared Prepare / Collect implementations
// -----------------------------------------------------------------------------

// prepareWithIssueMarkdown creates the workdir and writes ISSUE.md — the
// standard scaffold every Docker-sandboxed runner starts from. Idempotent.
func prepareWithIssueMarkdown(spec TaskSpec) (WorkDir, error) {
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

// collectWorkdirArtifacts walks the workdir and returns every non-hidden
// file as an Artifact. Kind is inferred from the extension.
// File contents are NOT read — only metadata.
func collectWorkdirArtifacts(wd WorkDir) ([]Artifact, error) {
	var out []Artifact
	err := filepath.WalkDir(wd.HostPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip; do not fail the whole collect
		}
		if d.IsDir() {
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

// artifactKind infers an ap_artifact.kind ("diff" | "log" | "file") from
// the filename extension. Unknown extensions map to "file".
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

// hasAnyKey reports whether m contains at least one of keys with a
// non-empty value.
func hasAnyKey(m map[string]string, keys ...string) bool {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != "" {
			return true
		}
	}
	return false
}
