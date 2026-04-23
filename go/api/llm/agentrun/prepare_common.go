package agentrun

import (
	"fmt"
	"strings"
)

// formatIssueMarkdown produces the standard ISSUE.md body every runner
// writes during Prepare. Kept stable so agents (and humans reviewing a
// workdir) always see the same layout.
func formatIssueMarkdown(spec TaskSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", spec.IssueTitle)
	if spec.IssueDesc != "" {
		b.WriteString(spec.IssueDesc)
		b.WriteString("\n\n")
	}
	if spec.Instructions != "" {
		b.WriteString("---\n\n## Agent Instructions\n\n")
		b.WriteString(spec.Instructions)
		b.WriteString("\n")
	}
	return b.String()
}
