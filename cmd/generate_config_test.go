package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kantis/dynatrace-tui/internal/config"
)

func TestWriteStarterConfigCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")

	if err := writeStarterConfig(path, false); err != nil {
		t.Fatalf("writeStarterConfig: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %o, want 0600", got)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(body), "environments:") {
		t.Errorf("scaffold missing environments key:\n%s", body)
	}
}

func TestWriteStarterConfigRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("existing: true\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := writeStarterConfig(path, false)
	if err == nil {
		t.Fatal("expected error when target exists, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %v, want substring 'already exists'", err)
	}

	body, _ := os.ReadFile(path)
	if string(body) != "existing: true\n" {
		t.Errorf("file was modified: %q", body)
	}
}

func TestWriteStarterConfigForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("existing: true\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := writeStarterConfig(path, true); err != nil {
		t.Fatalf("writeStarterConfig: %v", err)
	}

	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "environments:") {
		t.Errorf("scaffold did not overwrite existing file:\n%s", body)
	}
}

// The scaffold should parse cleanly via the real config loader so that
// stale comments/keys can't drift the template out of sync with the
// loader's expectations.
func TestWriteStarterConfigIsParseable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := writeStarterConfig(path, false); err != nil {
		t.Fatalf("writeStarterConfig: %v", err)
	}

	loaded, err := config.Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Names) != 1 || loaded.Names[0] != "PROD" {
		t.Errorf("Names = %v, want [PROD]", loaded.Names)
	}
}
