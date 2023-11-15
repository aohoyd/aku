package editor

import (
	"os"
	"strings"
)

// ResolveEditor returns the editor binary, checking KUBE_EDITOR, EDITOR, then vi.
func ResolveEditor() string {
	if e := os.Getenv("KUBE_EDITOR"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

// StripComments removes leading comment lines at the top of the file
// (before the first non-comment line) and any blank lines that follow them.
// Only top-of-file comments are stripped because FormatErrComment prepends
// errors there. Comment lines inside YAML block scalars (e.g. #!/bin/bash
// in ConfigMaps) are preserved.
func StripComments(s string) string {
	lines := strings.Split(s, "\n")
	i := 0
	for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "#") {
		i++
	}
	result := strings.Join(lines[i:], "\n")
	return strings.TrimLeft(result, "\n")
}

// FormatErrComment formats an error as YAML comment lines.
func FormatErrComment(err error) string {
	var b strings.Builder
	b.WriteString("# Error from server:\n")
	for line := range strings.SplitSeq(err.Error(), "\n") {
		b.WriteString("# ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("# Please fix the error above and save, or save unchanged to cancel.")
	return b.String()
}
