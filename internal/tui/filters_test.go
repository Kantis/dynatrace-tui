package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kantis/dynatrace-tui/internal/dql"
)

func TestSavedFiltersRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	want := []SavedFilter{
		{
			Name:     "by service",
			Template: `filter dt.entity.service == "$service"`,
			Suggestions: map[string][]string{
				"service": {"frontend", "auth-service"},
			},
		},
		{
			Name:        "errors only",
			Template:    `filter loglevel == "ERROR"`,
			Suggestions: nil,
		},
	}

	if err := writeSavedFilters(want); err != nil {
		t.Fatalf("writeSavedFilters: %v", err)
	}
	got, err := loadSavedFilters()
	if err != nil {
		t.Fatalf("loadSavedFilters: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip mismatch:\n got: %#v\nwant: %#v", got, want)
	}

	// Verify the on-disk file uses the `fragments:` top-level key so old
	// `filters:` files don't accidentally still load.
	p, err := savedFiltersPath()
	if err != nil {
		t.Fatalf("savedFiltersPath: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "fragments:") {
		t.Errorf("expected on-disk YAML to contain `fragments:` key, got:\n%s", data)
	}
	if filepath.Base(p) != "fragments.yaml" {
		t.Errorf("expected file to be named fragments.yaml, got %s", filepath.Base(p))
	}
}

func TestLoadSavedFiltersMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := loadSavedFilters()
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil slice, got: %#v", got)
	}
}

func TestNormalizeTemplate(t *testing.T) {
	cases := map[string]string{
		``:                                  ``,
		`   `:                               ``,
		`filter x == 1`:                     `filter x == 1`,
		`  filter x == 1  `:                 `filter x == 1`,
		`| filter x == 1`:                   `filter x == 1`,
		`|| filter x == 1`:                  `filter x == 1`,
		`  |   filter x == 1  `:             `filter x == 1`,
		`|filter x == 1`:                    `filter x == 1`,
	}
	for in, want := range cases {
		if got := normalizeTemplate(in); got != want {
			t.Errorf("normalizeTemplate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInsertFilterIntoEditor(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		fragment string
		want     string
	}{
		{"empty editor, bare fragment", "", `x == 1`, `x == 1`},
		{"empty editor, fragment with filter verb", "", `filter x == 1`, `filter x == 1`},
		{"non-empty editor, bare fragment", `from:now()-15m`, `x == 1`, "from:now()-15m\n| x == 1"},
		{"non-empty editor, fragment with filter verb", `from:now()-15m`, `filter x == 1`, "from:now()-15m\n| filter x == 1"},
		{"non-empty editor, sort fragment", `from:now()-15m`, `sort timestamp desc`, "from:now()-15m\n| sort timestamp desc"},
		{"trailing whitespace trimmed", `from:now()-15m   `, `filter x == 1`, "from:now()-15m\n| filter x == 1"},
		{"trailing newline trimmed", "from:now()-15m\n", `filter x == 1`, "from:now()-15m\n| filter x == 1"},
		{"fragment whitespace trimmed", `from:now()-15m`, `  filter x == 1  `, "from:now()-15m\n| filter x == 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := Model{editor: NewEditor(false)}
			m.editor.SetValue(tc.body)
			m = m.insertFilterIntoEditor(tc.fragment)
			if got := m.editor.Value(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveFilterSubstitutesProvidedValues(t *testing.T) {
	f := SavedFilter{
		Name:     "by-service",
		Template: `filter dt.entity.service == "$service" and loglevel == "$level"`,
	}
	m := Model{editor: NewEditor(false)}
	m = m.pickFilter(f)

	// Sanity check: pickFilter should have transitioned to the resolve modal
	// with one input per placeholder.
	if m.modal != modalResolveFilter {
		t.Fatalf("expected modal to be modalResolveFilter, got %v", m.modal)
	}
	if got := len(m.resolveInputs); got != 2 {
		t.Fatalf("expected 2 resolve inputs, got %d", got)
	}

	// Fill both placeholders and confirm.
	m.resolveInputs[0].SetValue("frontend")
	m.resolveInputs[1].SetValue("ERROR")
	m, _ = m.confirmResolveFilter()

	want := `filter dt.entity.service == "frontend" and loglevel == "ERROR"`
	if got := m.editor.Value(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveFilterKeepsBlankPlaceholderLiteral(t *testing.T) {
	f := SavedFilter{
		Name:     "two-params",
		Template: `filter a == "$x" and b == "$y"`,
	}
	m := Model{editor: NewEditor(false)}
	m = m.pickFilter(f)

	m.resolveInputs[0].SetValue("a-value")
	// Leave $y blank — it should remain literal.
	m.resolveInputs[1].SetValue("")
	m, _ = m.confirmResolveFilter()

	want := `filter a == "a-value" and b == "$y"`
	if got := m.editor.Value(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPickFilterNoPlaceholdersInsertsDirectly(t *testing.T) {
	f := SavedFilter{
		Name:     "no-params",
		Template: `filter loglevel == "ERROR"`,
	}
	m := Model{editor: NewEditor(false)}
	m.editor.SetValue("from:now()-15m")
	m = m.pickFilter(f)

	if m.modal != modalNone {
		t.Errorf("expected modal to be closed, got %v", m.modal)
	}
	want := "from:now()-15m\n| filter loglevel == \"ERROR\""
	if got := m.editor.Value(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPickFilterInsertsFragmentVerbatim(t *testing.T) {
	f := SavedFilter{
		Name:     "sort-by-time",
		Template: `sort timestamp desc`,
	}
	m := Model{editor: NewEditor(false)}
	m.editor.SetValue("from:now()-15m")
	m = m.pickFilter(f)

	if m.modal != modalNone {
		t.Errorf("expected modal to be closed, got %v", m.modal)
	}
	want := "from:now()-15m\n| sort timestamp desc"
	if got := m.editor.Value(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCycleResolveSuggestionFillsInput(t *testing.T) {
	f := SavedFilter{
		Name:     "by-level",
		Template: `filter loglevel == "$level"`,
		Suggestions: map[string][]string{
			"level": {"ERROR", "WARN", "INFO"},
		},
	}
	m := Model{editor: NewEditor(false)}
	m = m.pickFilter(f)

	// Down once → ERROR (idx 0).
	m = m.cycleResolveSuggestion(+1)
	if got := m.resolveInputs[0].Value(); got != "ERROR" {
		t.Errorf("after first ↓: got %q, want ERROR", got)
	}
	// Down again → WARN.
	m = m.cycleResolveSuggestion(+1)
	if got := m.resolveInputs[0].Value(); got != "WARN" {
		t.Errorf("after second ↓: got %q, want WARN", got)
	}
	// Up wraps from WARN → ERROR.
	m = m.cycleResolveSuggestion(-1)
	if got := m.resolveInputs[0].Value(); got != "ERROR" {
		t.Errorf("after ↑: got %q, want ERROR", got)
	}
}

// Sanity: dql.Placeholders/Substitute integration.
func TestDQLPlaceholdersIntegration(t *testing.T) {
	tmpl := `filter a == "$x" and b == "$y"`
	names := dql.Placeholders(tmpl)
	if !reflect.DeepEqual(names, []string{"x", "y"}) {
		t.Fatalf("unexpected placeholders: %v", names)
	}
	got := dql.Substitute(tmpl, map[string]string{"x": "1"})
	want := `filter a == "1" and b == "$y"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

