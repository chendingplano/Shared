package agentrun

import (
	"fmt"
	"os"
)

// NewRunnerByKind resolves a Runner for the given ap_agent.runtime_kind.
//
// Override: if AGENT_PLATFORM_FORCE_DRYRUN=true is set in the server env,
// every kind resolves to DryRunRunner. This lets us exercise the pipeline
// in M1b before real agent runners land in M1c.
//
// Unknown kinds return an error (the worker marks the task failed).
func NewRunnerByKind(kind string) (Runner, error) {
	if os.Getenv("AGENT_PLATFORM_FORCE_DRYRUN") == "true" {
		return DryRunRunner{}, nil
	}
	switch kind {
	case "dryrun":
		return DryRunRunner{}, nil
	case "claude_code":
		return ClaudeCodeRunner{}, nil
	case "codex", "openclaw", "opencode":
		return nil, fmt.Errorf("runner for %q not yet implemented; "+
			"set AGENT_PLATFORM_FORCE_DRYRUN=true to exercise the pipeline", kind)
	default:
		return nil, fmt.Errorf("unknown runtime kind %q", kind)
	}
}
