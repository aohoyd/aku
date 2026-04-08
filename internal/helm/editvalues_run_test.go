package helm

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type fakeHelmClient struct {
	values    map[string]any
	upgradeErr error
}

func (f *fakeHelmClient) ListReleases(namespace string) ([]ReleaseInfo, error) { return nil, nil }
func (f *fakeHelmClient) GetRelease(name, namespace string) (*ReleaseInfo, error) { return nil, nil }
func (f *fakeHelmClient) GetValues(name, namespace string) (map[string]any, error) {
	return f.values, nil
}
func (f *fakeHelmClient) History(name, namespace string) ([]RevisionInfo, error) { return nil, nil }
func (f *fakeHelmClient) Upgrade(name, namespace string, values map[string]any) error {
	return f.upgradeErr
}
func (f *fakeHelmClient) Rollback(name, namespace string, revision int) error { return nil }
func (f *fakeHelmClient) Uninstall(name, namespace string) error               { return nil }

func writeHelmEditorScript(t *testing.T, counterPath, modifiedContent string) string {
	t.Helper()
	scriptPath := filepath.Join(t.TempDir(), "editor.sh")
	script := fmt.Sprintf(`#!/bin/sh
count=$(cat %q 2>/dev/null || echo 0)
count=$((count + 1))
printf '%%s' "$count" > %q
if [ "$count" = "1" ]; then
    cat > "$1" << 'CONTENT'
%s
CONTENT
fi
`, counterPath, counterPath, modifiedContent)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return scriptPath
}

func TestEditValuesCancelAfterUpgradeError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on Windows")
	}

	client := &fakeHelmClient{
		values:     map[string]any{"replicaCount": 3},
		upgradeErr: fmt.Errorf("upgrade failed: chart validation error"),
	}

	counterPath := filepath.Join(t.TempDir(), "counter")
	editorScript := writeHelmEditorScript(t, counterPath, "replicaCount: 5\n")
	t.Setenv("EDITOR", editorScript)

	ec := &editValuesCommand{
		helmClient:  client,
		releaseName: "myrelease",
		namespace:   "default",
		stdin:       os.Stdin,
		stdout:      os.Stdout,
		stderr:      os.Stderr,
	}

	err := ec.Run()
	if err != nil {
		t.Fatalf("expected nil (cancel), got: %v", err)
	}

	countBytes, _ := os.ReadFile(counterPath)
	if string(countBytes) != "2" {
		t.Fatalf("expected editor to be called 2 times, got %s", countBytes)
	}
}

func TestEditValuesRefetchOnUpgradeError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on Windows")
	}

	// Client returns updated values on second GetValues call (simulating external change).
	client := &fakeHelmClient{
		values:     map[string]any{"replicaCount": 10, "image": "nginx:latest"},
		upgradeErr: fmt.Errorf("upgrade failed"),
	}

	counterPath := filepath.Join(t.TempDir(), "counter")
	retryContentPath := filepath.Join(t.TempDir(), "retry_content")

	scriptPath := filepath.Join(t.TempDir(), "editor.sh")
	script := fmt.Sprintf(`#!/bin/sh
count=$(cat %q 2>/dev/null || echo 0)
count=$((count + 1))
printf '%%s' "$count" > %q
if [ "$count" = "1" ]; then
    cat > "$1" << 'CONTENT'
replicaCount: 5
CONTENT
else
    cp "$1" %q
fi
`, counterPath, counterPath, retryContentPath)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", scriptPath)

	ec := &editValuesCommand{
		helmClient:  client,
		releaseName: "myrelease",
		namespace:   "default",
		stdin:       os.Stdin,
		stdout:      os.Stdout,
		stderr:      os.Stderr,
	}

	err := ec.Run()
	if err != nil {
		t.Fatalf("expected nil (cancel), got: %v", err)
	}

	// Verify the retry content contains the re-fetched values.
	retryContent, err := os.ReadFile(retryContentPath)
	if err != nil {
		t.Fatalf("failed to read retry content: %v", err)
	}
	content := string(retryContent)
	if !strings.Contains(content, "replicaCount: 10") {
		t.Errorf("expected retry content to contain re-fetched value 'replicaCount: 10', got:\n%s", content)
	}
	if !strings.Contains(content, "nginx:latest") {
		t.Errorf("expected retry content to contain re-fetched value 'nginx:latest', got:\n%s", content)
	}
}
