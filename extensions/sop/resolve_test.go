package sop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setConfigHome overrides XDG_CONFIG_HOME for the duration of the test.
// This lets EnsureGlobalDefault and Resolve use a temp directory without
// touching the real ~/.config/piglet/.
func setConfigHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", dir)
}

func TestResolve_ProjectFile(t *testing.T) {
	tmp := t.TempDir()
	projectSOP := filepath.Join(tmp, ".piglet", "sops", "plan.md")
	if err := os.MkdirAll(filepath.Dir(projectSOP), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectSOP, []byte("project content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Global config also present — project must win.
	setConfigHome(t, tmp)
	globalSOP := filepath.Join(tmp, "piglet", "sops", "plan.md")
	if err := os.MkdirAll(filepath.Dir(globalSOP), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalSOP, []byte("global content"), 0o644); err != nil {
		t.Fatal(err)
	}

	content, source, err := Resolve(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "project" {
		t.Errorf("source = %q, want %q", source, "project")
	}
	if content != "project content" {
		t.Errorf("content = %q, want %q", content, "project content")
	}
}

func TestResolve_GlobalFile(t *testing.T) {
	tmp := t.TempDir()
	setConfigHome(t, tmp)

	globalSOP := filepath.Join(tmp, "piglet", "sops", "plan.md")
	if err := os.MkdirAll(filepath.Dir(globalSOP), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalSOP, []byte("global content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// CWD has no .piglet/sops/plan.md.
	cwdTmp := t.TempDir()
	content, source, err := Resolve(cwdTmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "global" {
		t.Errorf("source = %q, want %q", source, "global")
	}
	if content != "global content" {
		t.Errorf("content = %q, want %q", content, "global content")
	}
}

func TestResolve_EmbeddedDefault(t *testing.T) {
	// Empty XDG_CONFIG_HOME pointing to a fresh temp dir — no files anywhere.
	tmp := t.TempDir()
	setConfigHome(t, tmp)

	cwd := t.TempDir()
	content, source, err := Resolve(cwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "embedded" {
		t.Errorf("source = %q, want %q", source, "embedded")
	}
	if content != defaultPlan {
		t.Errorf("content differs from embedded default")
	}
}

func TestResolve_EmptyCWD_SkipsProjectTier(t *testing.T) {
	// No global file either — should fall through to embedded.
	tmp := t.TempDir()
	setConfigHome(t, tmp)

	content, source, err := Resolve("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "embedded" {
		t.Errorf("source = %q, want %q", source, "embedded")
	}
	if content != defaultPlan {
		t.Errorf("content differs from embedded default")
	}
}

func TestEnsureGlobalDefault_CreatesFile(t *testing.T) {
	tmp := t.TempDir()
	setConfigHome(t, tmp)

	// File must not exist yet.
	globalSOP := filepath.Join(tmp, "piglet", "sops", "plan.md")
	if _, err := os.Stat(globalSOP); err == nil {
		t.Fatal("expected file to not exist before EnsureGlobalDefault")
	}

	if err := EnsureGlobalDefault(); err != nil {
		t.Fatalf("EnsureGlobalDefault: %v", err)
	}

	data, err := os.ReadFile(globalSOP)
	if err != nil {
		t.Fatalf("read after EnsureGlobalDefault: %v", err)
	}
	if !strings.Contains(string(data), "Phase 1") {
		t.Errorf("written content does not look like a planning SOP")
	}
}

func TestEnsureGlobalDefault_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	setConfigHome(t, tmp)

	// First call creates the file.
	if err := EnsureGlobalDefault(); err != nil {
		t.Fatalf("first call: %v", err)
	}

	globalSOP := filepath.Join(tmp, "piglet", "sops", "plan.md")
	// Overwrite with sentinel so we can detect whether second call clobbers it.
	if err := os.WriteFile(globalSOP, []byte("user edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Second call must be a no-op — must not overwrite the user's edits.
	if err := EnsureGlobalDefault(); err != nil {
		t.Fatalf("second call: %v", err)
	}

	data, err := os.ReadFile(globalSOP)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "user edited" {
		t.Errorf("second call overwrote user content; got %q", string(data))
	}
}
