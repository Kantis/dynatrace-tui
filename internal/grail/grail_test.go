package grail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type stubTokens struct{ tok string }

func (s stubTokens) Token(_ context.Context) (string, error) { return s.tok, nil }

func newTestClient(t *testing.T, h http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := &Client{
		BaseURL:    srv.URL,
		Tokens:     stubTokens{tok: "test-token"},
		HTTPClient: srv.Client(),
	}
	return c, srv
}

func TestExecuteAndPollUntilDone(t *testing.T) {
	var polls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/query:execute", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("missing/wrong auth header: %q", got)
		}
		var body ExecuteRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if !strings.Contains(body.Query, "fetch logs") {
			t.Errorf("query body did not contain DQL: %q", body.Query)
		}
		_ = json.NewEncoder(w).Encode(Response{State: StateRunning, RequestToken: "tok-123"})
	})
	mux.HandleFunc("/query:poll", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("request-token") != "tok-123" {
			t.Errorf("missing request-token param: %s", r.URL.RawQuery)
		}
		n := atomic.AddInt32(&polls, 1)
		if n < 2 {
			_ = json.NewEncoder(w).Encode(Response{State: StateRunning, RequestToken: "tok-123"})
			return
		}
		_ = json.NewEncoder(w).Encode(Response{
			State:        StateSucceeded,
			RequestToken: "tok-123",
			Result: &QueryResult{
				Records: Records{{"timestamp": "2026-04-28T12:00:00Z", "content": "hello"}},
			},
		})
	})
	c, _ := newTestClient(t, mux)

	ctx := context.Background()
	exec, err := c.Execute(ctx, ExecuteRequest{Query: "fetch logs | limit 5"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exec.RequestToken != "tok-123" {
		t.Fatalf("unexpected token: %q", exec.RequestToken)
	}
	final, err := c.PollUntilDone(ctx, exec.RequestToken)
	if err != nil {
		t.Fatalf("PollUntilDone: %v", err)
	}
	if final.State != StateSucceeded {
		t.Fatalf("final state = %q, want SUCCEEDED", final.State)
	}
	if final.Result == nil || len(final.Result.Records) != 1 {
		t.Fatalf("expected 1 record, got %+v", final.Result)
	}
	if got := final.Result.Records[0]["content"]; got != "hello" {
		t.Errorf("record content = %v", got)
	}
	if atomic.LoadInt32(&polls) < 2 {
		t.Errorf("expected at least 2 polls, got %d", polls)
	}
}

func TestCancel(t *testing.T) {
	var cancelled int32
	mux := http.NewServeMux()
	mux.HandleFunc("/query:cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("cancel method = %s, want POST", r.Method)
		}
		if r.URL.Query().Get("request-token") != "tok-xyz" {
			t.Errorf("missing request-token: %s", r.URL.RawQuery)
		}
		atomic.AddInt32(&cancelled, 1)
		w.WriteHeader(http.StatusOK)
	})
	c, _ := newTestClient(t, mux)
	if err := c.Cancel(context.Background(), "tok-xyz"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if atomic.LoadInt32(&cancelled) != 1 {
		t.Errorf("cancel handler called %d times", cancelled)
	}
}

func TestPollFailedState(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/query:poll", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Response{
			State:        StateFailed,
			RequestToken: "tok-fail",
			Error:        &APIError{Code: 400, Message: "bad query"},
		})
	})
	c, _ := newTestClient(t, mux)
	resp, err := c.PollUntilDone(context.Background(), "tok-fail")
	if err != nil {
		t.Fatalf("PollUntilDone: %v", err)
	}
	if resp.State != StateFailed {
		t.Fatalf("state = %q", resp.State)
	}
	if resp.Error == nil || resp.Error.Code != 400 {
		t.Fatalf("expected APIError with code 400, got %+v", resp.Error)
	}
}

func TestServerErrorPropagates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/query:execute", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	c, _ := newTestClient(t, mux)
	_, err := c.Execute(context.Background(), ExecuteRequest{Query: "fetch logs"})
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error did not mention 401: %v", err)
	}
}
