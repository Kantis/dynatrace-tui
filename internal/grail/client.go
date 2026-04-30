package grail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

type Client struct {
	BaseURL    string
	EnvID      string
	Tokens     TokenProvider
	HTTPClient *http.Client
}

// State of a Grail query.
type State string

const (
	StateRunning   State = "RUNNING"
	StateSucceeded State = "SUCCEEDED"
	StateFailed    State = "FAILED"
	StateCancelled State = "CANCELLED"
)

// Records is the shape returned by Grail under result.records.
type Records []map[string]any

type QueryResult struct {
	Records  Records         `json:"records"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// FieldOrder returns the column names in the order Grail emitted them in the
// response metadata. Records arrive as map[string]any, so this is the only way
// to recover the projection order from `| fields …`, `summarize`, etc.
//
// Grail's metadata shape is `{"types":[{"mappings":{"<name>":{...},...}}]}`.
// JSON object key order isn't preserved by Go's map decoder, so we stream-parse
// the raw bytes via json.Decoder.Token() to read keys in document order.
//
// Returns nil when metadata is missing or doesn't match the expected shape;
// callers fall back to their own ordering heuristic.
func (qr *QueryResult) FieldOrder() []string {
	if qr == nil || len(qr.Metadata) == 0 {
		return nil
	}
	var meta struct {
		Types []json.RawMessage `json:"types"`
	}
	if err := json.Unmarshal(qr.Metadata, &meta); err != nil || len(meta.Types) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range meta.Types {
		var typed struct {
			Mappings json.RawMessage `json:"mappings"`
		}
		if err := json.Unmarshal(t, &typed); err != nil || len(typed.Mappings) == 0 {
			continue
		}
		for _, k := range jsonObjectKeys(typed.Mappings) {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

// jsonObjectKeys reads the top-level keys of a JSON object in document order.
// Returns nil if raw is not a JSON object.
func jsonObjectKeys(raw json.RawMessage) []string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil
		}
		key, ok := tok.(string)
		if !ok {
			return nil
		}
		keys = append(keys, key)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil
		}
	}
	return keys
}

// Response is the envelope returned by query:execute and query:poll.
type Response struct {
	State        State        `json:"state"`
	RequestToken string       `json:"requestToken"`
	Result       *QueryResult `json:"result,omitempty"`
	Error        *APIError    `json:"error,omitempty"`
}

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("grail error %d: %s", e.Code, e.Message)
}

// New builds a client targeting the given Dynatrace environment id (e.g. "abc12345").
func New(envID string, tp TokenProvider) *Client {
	return &Client{
		BaseURL:    fmt.Sprintf("https://%s.apps.dynatrace.com/platform/storage/query/v1", envID),
		EnvID:      envID,
		Tokens:     tp,
		HTTPClient: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any) (*Response, error) {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("grail %s %s: %s: %s", method, path, resp.Status, string(raw))
	}
	var out Response
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("decode %s response: %w", path, err)
		}
	}
	return &out, nil
}
