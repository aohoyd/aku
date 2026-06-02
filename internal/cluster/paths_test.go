package cluster

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDefaultKubeconfigFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	canonical := filepath.Join(home, ".kube", "config")

	t.Run("unset env, no flag -> canonical only", func(t *testing.T) {
		t.Setenv("KUBECONFIG", "")
		got := DefaultKubeconfigFiles("")
		if want := []string{canonical}; !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("single active file -> active then canonical", func(t *testing.T) {
		active := filepath.Join(home, "configs", "active.yaml")
		t.Setenv("KUBECONFIG", active)
		got := DefaultKubeconfigFiles("")
		if want := []string{active, canonical}; !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v (canonical must always be included)", got, want)
		}
	})

	t.Run("colon list -> each entry then canonical, in order", func(t *testing.T) {
		a := filepath.Join(home, "a.yaml")
		b := filepath.Join(home, "b.yaml")
		t.Setenv("KUBECONFIG", strings.Join([]string{a, b}, string(os.PathListSeparator)))
		got := DefaultKubeconfigFiles("")
		if want := []string{a, b, canonical}; !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("flag wins precedence and is first", func(t *testing.T) {
		flag := filepath.Join(home, "flag.yaml")
		env := filepath.Join(home, "env.yaml")
		t.Setenv("KUBECONFIG", env)
		got := DefaultKubeconfigFiles(flag)
		if want := []string{flag, env, canonical}; !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("dedupe when KUBECONFIG already is canonical", func(t *testing.T) {
		t.Setenv("KUBECONFIG", canonical)
		got := DefaultKubeconfigFiles("")
		if want := []string{canonical}; !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v (no duplicate canonical)", got, want)
		}
	})

	t.Run("tilde and env expansion applied", func(t *testing.T) {
		t.Setenv("AKU_TEST_KC_DIR", filepath.Join(home, "envdir"))
		t.Setenv("KUBECONFIG", "$AKU_TEST_KC_DIR/c.yaml")
		got := DefaultKubeconfigFiles("~/flag.yaml")
		want := []string{
			filepath.Join(home, "flag.yaml"),
			filepath.Join(home, "envdir", "c.yaml"),
			canonical,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}
