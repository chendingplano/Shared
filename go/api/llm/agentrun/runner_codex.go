package agentrun

import "context"

const (
	defaultCodexImage = "chenweb/agentrun-codex:v1"
	codexImageEnvVar  = "AGENT_PLATFORM_CODEX_IMAGE"
)

// codexConfig is the DockerRunnerConfig for OpenAI's Codex CLI
// (`@openai/codex`). The container reads /workspace/ISSUE.md and hands
// it to `codex exec` in non-interactive mode.
//
// Dockerfile: ChenWeb/docker/agentrun-codex/Dockerfile.
var codexConfig = DockerRunnerConfig{
	Kind:         "codex",
	DefaultImage: defaultCodexImage,
	ImageEnvVar:  codexImageEnvVar,
	RequiredKeys: []string{"OPENAI_API_KEY"},
}

// CodexRunner runs the Codex CLI in a sandboxed container.
type CodexRunner struct{}

func (CodexRunner) Kind() string    { return codexConfig.Kind }
func (CodexRunner) Version() string { return codexConfig.ResolveImage() }

func (CodexRunner) Prepare(_ context.Context, spec TaskSpec) (WorkDir, error) {
	return prepareWithIssueMarkdown(spec)
}

func (CodexRunner) Run(ctx context.Context, wd WorkDir, spec TaskSpec, out chan<- Event) error {
	return codexConfig.runSandboxed(ctx, wd, spec, out)
}

func (CodexRunner) Collect(_ context.Context, wd WorkDir) ([]Artifact, error) {
	return collectWorkdirArtifacts(wd)
}
