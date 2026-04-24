package agentrun

import "context"

const (
	defaultOpenCodeImage = "chenweb/agentrun-opencode:v1"
	openCodeImageEnvVar  = "AGENT_PLATFORM_OPENCODE_IMAGE"
)

// opencodeConfig is the DockerRunnerConfig for the OpenCode CLI
// (SST's agent — https://opencode.ai). The container reads
// /workspace/ISSUE.md and hands it to `opencode run` non-interactively.
//
// Dockerfile: ChenWeb/docker/agentrun-opencode/Dockerfile.
var opencodeConfig = DockerRunnerConfig{
	Kind:         "opencode",
	DefaultImage: defaultOpenCodeImage,
	ImageEnvVar:  openCodeImageEnvVar,
	RequiredKeys: []string{"OPENAI_API_KEY"},
}

// OpenCodeRunner runs the OpenCode CLI in a sandboxed container.
type OpenCodeRunner struct{}

func (OpenCodeRunner) Kind() string    { return opencodeConfig.Kind }
func (OpenCodeRunner) Version() string { return opencodeConfig.ResolveImage() }

func (OpenCodeRunner) Prepare(_ context.Context, spec TaskSpec) (WorkDir, error) {
	return prepareWithIssueMarkdown(spec)
}

func (OpenCodeRunner) Run(ctx context.Context, wd WorkDir, spec TaskSpec, out chan<- Event) error {
	return opencodeConfig.runSandboxed(ctx, wd, spec, out)
}

func (OpenCodeRunner) Collect(_ context.Context, wd WorkDir) ([]Artifact, error) {
	return collectWorkdirArtifacts(wd)
}
