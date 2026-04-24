package agentrun

import "context"

const (
	defaultClaudeImage = "chenweb/agentrun-claude:v1"
	claudeImageEnvVar  = "AGENT_PLATFORM_CLAUDE_IMAGE"
)

// claudeConfig is the DockerRunnerConfig for the Claude Code CLI.
// The container reads /workspace/ISSUE.md and calls `claude --print`.
// Dockerfile: ChenWeb/docker/agentrun-claude/Dockerfile.
var claudeConfig = DockerRunnerConfig{
	Kind:         "claude_code",
	DefaultImage: defaultClaudeImage,
	ImageEnvVar:  claudeImageEnvVar,
	RequiredKeys: []string{"ANTHROPIC_API_KEY", "CLAUDE_API_KEY"},
}

// ClaudeCodeRunner runs Anthropic's Claude Code CLI in a sandboxed container.
type ClaudeCodeRunner struct{}

func (ClaudeCodeRunner) Kind() string    { return claudeConfig.Kind }
func (ClaudeCodeRunner) Version() string { return claudeConfig.ResolveImage() }

func (ClaudeCodeRunner) Prepare(_ context.Context, spec TaskSpec) (WorkDir, error) {
	return prepareWithIssueMarkdown(spec)
}

func (ClaudeCodeRunner) Run(ctx context.Context, wd WorkDir, spec TaskSpec, out chan<- Event) error {
	return claudeConfig.runSandboxed(ctx, wd, spec, out)
}

func (ClaudeCodeRunner) Collect(_ context.Context, wd WorkDir) ([]Artifact, error) {
	return collectWorkdirArtifacts(wd)
}
