package helm

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/editor"
	"github.com/aohoyd/aku/internal/msgs"
	sigsyaml "sigs.k8s.io/yaml"
)

func marshalValues(values map[string]any) ([]byte, error) {
	if len(values) == 0 {
		return []byte("# No user-supplied values\n"), nil
	}
	return sigsyaml.Marshal(values)
}

type editValuesCommand struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	helmClient  Client
	releaseName string
	namespace   string
}

func (e *editValuesCommand) SetStdin(r io.Reader)  { e.stdin = r }
func (e *editValuesCommand) SetStdout(w io.Writer) { e.stdout = w }
func (e *editValuesCommand) SetStderr(w io.Writer) { e.stderr = w }

func (e *editValuesCommand) Run() error {
	values, err := e.helmClient.GetValues(e.releaseName, e.namespace)
	if err != nil {
		return fmt.Errorf("get values: %w", err)
	}

	yamlBytes, err := marshalValues(values)
	if err != nil {
		return fmt.Errorf("marshal values: %w", err)
	}
	baseContent := string(yamlBytes)
	baseHash := sha256.Sum256([]byte(baseContent))

	tmpFile, err := os.CreateTemp("", "ktui-helm-values-*.yaml")
	if err != nil {
		return fmt.Errorf("tmpfile: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(baseContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write: %w", err)
	}
	tmpFile.Close()

	for {
		editorBin := editor.ResolveEditor()
		parts := strings.Fields(editorBin)
		parts = append(parts, tmpPath)
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Stdin = e.stdin
		cmd.Stdout = e.stdout
		cmd.Stderr = e.stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("editor: %w", err)
		}

		edited, err := os.ReadFile(tmpPath)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		cleaned := editor.StripComments(string(edited))
		if strings.TrimSpace(cleaned) == "" {
			return nil // cancelled
		}
		cleanedHash := sha256.Sum256([]byte(cleaned))
		if cleanedHash == baseHash {
			return nil // unchanged
		}

		var newValues map[string]any
		if err := sigsyaml.Unmarshal([]byte(cleaned), &newValues); err != nil {
			// Parse error: re-open with base content
			content := editor.FormatErrComment(err) + "\n" + baseContent
			if writeErr := os.WriteFile(tmpPath, []byte(content), 0600); writeErr != nil {
				return fmt.Errorf("write retry: %w", writeErr)
			}
			continue
		}

		if err := e.helmClient.Upgrade(e.releaseName, e.namespace, newValues); err != nil {
			// Re-fetch latest values from the server.
			freshValues, getErr := e.helmClient.GetValues(e.releaseName, e.namespace)
			if getErr == nil {
				if freshYAML, marshalErr := marshalValues(freshValues); marshalErr == nil {
					baseContent = string(freshYAML)
					baseHash = sha256.Sum256([]byte(baseContent))
				}
			}
			content := editor.FormatErrComment(err) + "\n" + baseContent
			if writeErr := os.WriteFile(tmpPath, []byte(content), 0600); writeErr != nil {
				return fmt.Errorf("write retry: %w", writeErr)
			}
			continue
		}

		return nil
	}
}

// EditValuesCmd returns a tea.Cmd that suspends the TUI, opens the release
// values in an editor, and performs helm upgrade on save.
func EditValuesCmd(helmClient Client, name, namespace string) tea.Cmd {
	ec := &editValuesCommand{
		helmClient:  helmClient,
		releaseName: name,
		namespace:   namespace,
	}
	return tea.Exec(ec, func(err error) tea.Msg {
		if err != nil {
			return msgs.ActionResultMsg{Err: err}
		}
		return msgs.ActionResultMsg{ActionID: "helm-edit-values:" + name}
	})
}
