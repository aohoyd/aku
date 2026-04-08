package k8s

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/editor"
	"github.com/aohoyd/aku/internal/msgs"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	sigsyaml "sigs.k8s.io/yaml"
)

// marshalForEdit produces clean YAML from an Unstructured object,
// stripping managedFields. The original object is not mutated.
func marshalForEdit(obj *unstructured.Unstructured) ([]byte, error) {
	clean := obj.DeepCopy()
	if md, ok := clean.Object["metadata"].(map[string]any); ok {
		delete(md, "managedFields")
	}
	return sigsyaml.Marshal(clean.Object)
}

// editCommand implements tea.ExecCommand for the edit-retry loop.
type editCommand struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	obj           *unstructured.Unstructured
	gvr           schema.GroupVersionResource
	clusterScoped bool
	dynClient     dynamic.Interface
}

func (e *editCommand) SetStdin(r io.Reader)  { e.stdin = r }
func (e *editCommand) SetStdout(w io.Writer) { e.stdout = w }
func (e *editCommand) SetStderr(w io.Writer) { e.stderr = w }

// Run executes the edit loop: marshal -> editor -> apply -> retry on error.
// Returns nil on success or cancel, error only for unrecoverable failures.
func (e *editCommand) Run() error {
	yamlBytes, err := marshalForEdit(e.obj)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	baseContent := string(yamlBytes)
	baseHash := sha256.Sum256([]byte(baseContent))

	tmpFile, err := os.CreateTemp("", "ktui-edit-*.yaml")
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

	// Build resource interface for re-fetching on conflict.
	var fetchRes dynamic.ResourceInterface
	if e.clusterScoped {
		fetchRes = e.dynClient.Resource(e.gvr)
	} else {
		fetchRes = e.dynClient.Resource(e.gvr).Namespace(e.obj.GetNamespace())
	}

	for {
		// Open editor
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

		// Read edited content
		edited, err := os.ReadFile(tmpPath)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		// Strip comments and check for cancel
		cleaned := editor.StripComments(string(edited))
		if strings.TrimSpace(cleaned) == "" {
			return nil // cancelled: empty
		}
		cleanedHash := sha256.Sum256([]byte(cleaned))
		if cleanedHash == baseHash {
			return nil // cancelled: unchanged from base
		}

		// Parse YAML
		var newObj unstructured.Unstructured
		if err := sigsyaml.Unmarshal([]byte(cleaned), &newObj.Object); err != nil {
			// Parse error: prepend error and re-open with base content
			content := editor.FormatErrComment(err) + "\n" + baseContent
			if writeErr := os.WriteFile(tmpPath, []byte(content), 0600); writeErr != nil {
				return fmt.Errorf("write retry: %w", writeErr)
			}
			continue
		}

		// Apply update
		var res dynamic.ResourceInterface
		if e.clusterScoped {
			res = e.dynClient.Resource(e.gvr)
		} else {
			ns := newObj.GetNamespace()
			if ns == "" {
				ns = e.obj.GetNamespace()
			}
			res = e.dynClient.Resource(e.gvr).Namespace(ns)
		}

		if _, err := res.Update(context.Background(), &newObj, metav1.UpdateOptions{}); err != nil {
			// On conflict, re-fetch the latest version from the server.
			if apierrors.IsConflict(err) {
				latest, getErr := fetchRes.Get(context.Background(), e.obj.GetName(), metav1.GetOptions{})
				if getErr == nil {
					if freshYAML, marshalErr := marshalForEdit(latest); marshalErr == nil {
						baseContent = string(freshYAML)
						baseHash = sha256.Sum256([]byte(baseContent))
					}
				}
			}
			// API error: prepend error and re-open with base content
			content := editor.FormatErrComment(err) + "\n" + baseContent
			if writeErr := os.WriteFile(tmpPath, []byte(content), 0600); writeErr != nil {
				return fmt.Errorf("write retry: %w", writeErr)
			}
			continue
		}

		return nil // success
	}
}

// EditCmd returns a tea.Cmd that suspends the TUI and opens the resource
// in an external editor with a retry loop on errors.
func EditCmd(dynClient dynamic.Interface, gvr schema.GroupVersionResource,
	clusterScoped bool, obj *unstructured.Unstructured) tea.Cmd {
	ec := &editCommand{
		obj:           obj,
		gvr:           gvr,
		clusterScoped: clusterScoped,
		dynClient:     dynClient,
	}
	return tea.Exec(ec, func(err error) tea.Msg {
		if err != nil {
			return msgs.ActionResultMsg{Err: err}
		}
		return msgs.ActionResultMsg{ActionID: "edit:" + obj.GetName()}
	})
}
