package editor

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveEditor(t *testing.T) {
	tests := []struct {
		name       string
		kubeEditor string
		editor     string
		want       string
	}{
		{"KUBE_EDITOR set", "nano", "vim", "nano"},
		{"EDITOR fallback", "", "vim", "vim"},
		{"vi default", "", "", "vi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KUBE_EDITOR", tt.kubeEditor)
			t.Setenv("EDITOR", tt.editor)
			if got := ResolveEditor(); got != tt.want {
				t.Errorf("ResolveEditor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripComments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no comments", "key: value\n", "key: value\n"},
		{"all leading comments", "# comment\n# another\n", ""},
		{"leading comment then yaml", "# comment\nkey: value\n", "key: value\n"},
		{"indented leading comment", "  # comment\nkey: value\n", "key: value\n"},
		{"hash in quoted value", "name: \"foo # bar\"\n", "name: \"foo # bar\"\n"},
		{"comment inside yaml preserved", "key: value\n# inline comment\nother: val\n", "key: value\n# inline comment\nother: val\n"},
		{"block scalar with shebang", "# error\ndata:\n  script: |\n    #!/bin/bash\n    echo hello\n", "data:\n  script: |\n    #!/bin/bash\n    echo hello\n"},
		{"blank line after comments stripped", "# comment\n\nkey: value\n", "key: value\n"},
		{"multiple blank lines after comments", "# comment\n\n\nkey: value\n", "key: value\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripComments(tt.input); got != tt.want {
				t.Errorf("StripComments() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatErrComment(t *testing.T) {
	err := errors.New("field spec.replicas is immutable\nsecond line")
	got := FormatErrComment(err)
	if !strings.HasPrefix(got, "# Error from server:") {
		t.Errorf("expected error header, got: %s", got)
	}
	if !strings.Contains(got, "# field spec.replicas is immutable") {
		t.Errorf("expected error line, got: %s", got)
	}
	if !strings.Contains(got, "# second line") {
		t.Errorf("expected second error line, got: %s", got)
	}
	if !strings.Contains(got, "save unchanged to cancel") {
		t.Errorf("expected cancel hint, got: %s", got)
	}
}

func TestRetryContentPreservesUserEdits(t *testing.T) {
	userEdits := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: edited\n"
	err := errors.New("field is immutable")
	content := FormatErrComment(err) + "\n" + userEdits

	recovered := StripComments(content)
	if recovered != userEdits {
		t.Fatalf("retry content should preserve user edits.\nwant: %q\ngot:  %q", userEdits, recovered)
	}
}

func TestRetryContentCancelDetection(t *testing.T) {
	original := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n"
	err := errors.New("some server error")
	retryContent := FormatErrComment(err) + "\n" + original

	recovered := StripComments(retryContent)
	if recovered != original {
		t.Fatalf("cancel detection should work on retry.\nwant: %q\ngot:  %q", original, recovered)
	}
}
