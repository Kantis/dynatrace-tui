package dql

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestNotebookURL(t *testing.T) {
	envID := "abc12345"
	query := `fetch logs, from:now()-15m | filter loglevel == "ERROR"`

	got := NotebookURL(envID, query)

	prefix := "https://abc12345.apps.dynatrace.com/ui/apps/dynatrace.notebooks/notebook/preset?"
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("URL prefix mismatch:\n got: %s\nwant: %s...", got, prefix)
	}

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	raw := u.Query().Get("notebook")
	if raw == "" {
		t.Fatal("notebook query param missing")
	}

	var spec struct {
		Sections []struct {
			Type string `json:"type"`
			DQL  struct {
				Value string `json:"value"`
			} `json:"dql"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatalf("decode notebook payload: %v\nraw: %s", err, raw)
	}
	if len(spec.Sections) != 1 {
		t.Fatalf("want 1 section, got %d", len(spec.Sections))
	}
	if spec.Sections[0].Type != "dql" {
		t.Errorf("section type: got %q, want %q", spec.Sections[0].Type, "dql")
	}
	if spec.Sections[0].DQL.Value != query {
		t.Errorf("query round-trip: got %q, want %q", spec.Sections[0].DQL.Value, query)
	}
}

func TestNotebookURLEnvIDInHost(t *testing.T) {
	got := NotebookURL("xyz99", "fetch logs")
	if !strings.HasPrefix(got, "https://xyz99.apps.dynatrace.com/") {
		t.Errorf("env ID not in host: %s", got)
	}
}

func TestNotebookURLEncodesSpecialChars(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"quotes", `fetch logs | filter content == "boom"`},
		{"backslash", `fetch logs | filter content matches "a\\b"`},
		{"multiline", "fetch logs\n| filter loglevel == \"WARN\"\n| limit 50"},
		{"ampersand", `fetch logs | filter url contains "a&b=c"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := NotebookURL("abc", tc.query)
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			payload := u.Query().Get("notebook")
			var spec struct {
				Sections []struct {
					DQL struct {
						Value string `json:"value"`
					} `json:"dql"`
				} `json:"sections"`
			}
			if err := json.Unmarshal([]byte(payload), &spec); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got := spec.Sections[0].DQL.Value; got != tc.query {
				t.Errorf("round-trip:\n got %q\nwant %q", got, tc.query)
			}
		})
	}
}
