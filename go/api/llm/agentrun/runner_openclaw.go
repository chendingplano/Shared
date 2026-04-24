package agentrun

import "context"

const (
	defaultOpenClawImage = "chenweb/agentrun-openclaw:v1"
	openClawImageEnvVar  = "AGENT_PLATFORM_OPENCLAW_IMAGE"
)

// openClawConfig is the DockerRunnerConfig for the OpenClaw CLI.
// The container reads /workspace/ISSUE.md and hands it to the CLI in
// non-interactive mode. OpenClaw speaks to Anthropic's API, so the
// required keys mirror Claude Code's.
//
// Dockerfile: ChenWeb/docker/agentrun-openclaw/Dockerfile.
var openClawConfig = DockerRunnerConfig{
	Kind:         "openclaw",
	DefaultImage: defaultOpenClawImage,
	ImageEnvVar:  openClawImageEnvVar,
	RequiredKeys: []string{"ANTHROPIC_API_KEY", "CLAUDE_API_KEY"},
}

// OpenClawRunner runs the OpenClaw CLI in a sandboxed container.
type OpenClawRunner struct{}

func (OpenClawRunner) Kind() string    { return openClawConfig.Kind }
func (OpenClawRunner) Version() string { return openClawConfig.ResolveImage() }

func (OpenClawRunner) Prepare(_ context.Context, spec TaskSpec) (WorkDir, error) {
	return prepareWithIssueMarkdown(spec)
}

func (OpenClawRunner) Run(ctx context.Context, wd WorkDir, spec TaskSpec, out chan<- Event) error {
	return openClawConfig.runSandboxed(ctx, wd, spec, out)
}

func (OpenClawRunner) Collect(_ context.Context, wd WorkDir) ([]Artifact, error) {
	return collectWorkdirArtifacts(wd)
}
