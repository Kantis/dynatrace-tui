package dql

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// NotebookURL returns a Dynatrace web-UI deep link that opens the given DQL
// query in the active tenant's Notebooks app as a single DQL section.
//
// The Notebooks preset reader expects a JSON-encoded notebook spec passed via
// the `notebook` query parameter. The minimal shape is one section of type
// `dql` carrying the query string.
func NotebookURL(envID, query string) string {
	preset := map[string]any{
		"defaultTimeframe": map[string]string{"from": "now()-2h", "to": "now()"},
		"sections": []map[string]any{
			{
				"type": "dql",
				"dql": map[string]any{
					"value": query,
				},
			},
		},
	}
	payload, _ := json.Marshal(preset)
	q := url.Values{}
	q.Set("notebook", string(payload))
	return fmt.Sprintf("https://%s.apps.dynatrace.com/ui/apps/dynatrace.notebooks/notebook/preset?%s", envID, q.Encode())
}
