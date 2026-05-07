package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadTimePicker(t *testing.T) {
	path := writeConfig(t, `
environments:
  default:
    environment_id: abc123
    platform_token: secret
time_picker:
  from:
    - now()-5m
    - now()-1h
    - start_of_hour
  to:
    - now()
`)
	loaded, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantFrom := []string{"now()-5m", "now()-1h", "start_of_hour"}
	if !reflect.DeepEqual(loaded.TimePickerFrom, wantFrom) {
		t.Errorf("TimePickerFrom = %v, want %v", loaded.TimePickerFrom, wantFrom)
	}
	wantTo := []string{"now()"}
	if !reflect.DeepEqual(loaded.TimePickerTo, wantTo) {
		t.Errorf("TimePickerTo = %v, want %v", loaded.TimePickerTo, wantTo)
	}
}

func TestLoadTimePickerUnset(t *testing.T) {
	// No `time_picker` block — both fields should remain nil so the consumer
	// uses the built-in defaults.
	path := writeConfig(t, `
environments:
  default:
    environment_id: abc123
    platform_token: secret
`)
	loaded, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.TimePickerFrom != nil {
		t.Errorf("TimePickerFrom = %v, want nil", loaded.TimePickerFrom)
	}
	if loaded.TimePickerTo != nil {
		t.Errorf("TimePickerTo = %v, want nil", loaded.TimePickerTo)
	}
}

func TestLoadSimplifiedPreviewDefaultsTrue(t *testing.T) {
	path := writeConfig(t, `
environments:
  default:
    environment_id: abc123
    platform_token: secret
`)
	loaded, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.SimplifiedPreviews {
		t.Errorf("SimplifiedPreviews = false, want true (default)")
	}
}

func TestLoadSimplifiedPreviewDisabled(t *testing.T) {
	path := writeConfig(t, `
environments:
  default:
    environment_id: abc123
    platform_token: secret
simplified_preview: false
`)
	loaded, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.SimplifiedPreviews {
		t.Errorf("SimplifiedPreviews = true, want false")
	}
}

func TestLoadTimePickerExplicitEmpty(t *testing.T) {
	// Explicit `from: []` is preserved as an empty (non-nil) slice so the
	// consumer can render an empty list rather than falling back to defaults.
	path := writeConfig(t, `
environments:
  default:
    environment_id: abc123
    platform_token: secret
time_picker:
  from: []
`)
	loaded, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.TimePickerFrom == nil {
		t.Errorf("TimePickerFrom is nil; want non-nil empty slice")
	}
	if len(loaded.TimePickerFrom) != 0 {
		t.Errorf("len(TimePickerFrom) = %d, want 0", len(loaded.TimePickerFrom))
	}
}
