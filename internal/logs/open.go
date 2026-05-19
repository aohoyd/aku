package logs

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/editor"
	"github.com/aohoyd/aku/internal/msgs"
)

// openCommand implements tea.ExecCommand to suspend the TUI and open a
// previously saved log file in the editor returned by editor.ResolveEditor
// (KUBE_EDITOR -> EDITOR -> vi). Unlike k8s.editCommand it does no
// apply/retry loop: once the editor exits we simply return its error.
type openCommand struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	path string
}

func (o *openCommand) SetStdin(r io.Reader)  { o.stdin = r }
func (o *openCommand) SetStdout(w io.Writer) { o.stdout = w }
func (o *openCommand) SetStderr(w io.Writer) { o.stderr = w }

func (o *openCommand) Run() error {
	editorBin := editor.ResolveEditor()
	parts := strings.Fields(editorBin)
	parts = append(parts, o.path)
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = o.stdin
	cmd.Stdout = o.stdout
	cmd.Stderr = o.stderr
	return cmd.Run()
}

// OpenCmd returns a tea.Cmd that suspends the TUI and opens path in the
// resolved editor. Mirrors k8s.EditCmd / k8s.ExecCmd factory pattern.
func OpenCmd(path string) tea.Cmd {
	ec := &openCommand{path: path}
	return tea.Exec(ec, func(err error) tea.Msg {
		if err != nil {
			return msgs.ErrMsg{Err: fmt.Errorf("editor failed: %w", err)}
		}
		return msgs.WarningMsg{Text: fmt.Sprintf("Editor exited (file kept at %s)", path)}
	})
}
