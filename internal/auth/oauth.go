package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const DefaultTokenURL = "https://sso.dynatrace.com/sso/oauth2/token"

// TokenSource fetches and caches OAuth2 client_credentials tokens.
type TokenSource struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
	HTTPClient   *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func New(clientID, clientSecret string, scopes []string) *TokenSource {
	return &TokenSource{
		TokenURL:     DefaultTokenURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopes,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Token returns a cached token if it is still fresh; otherwise fetches a new one.
// Refreshes 30s before expiry to avoid borderline expirations.
func (ts *TokenSource) Token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.token != "" && time.Until(ts.expires) > 30*time.Second {
		return ts.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", ts.ClientID)
	form.Set("client_secret", ts.ClientSecret)
	form.Set("scope", strings.Join(ts.Scopes, " "))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ts.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("token response missing access_token")
	}

	ts.token = tr.AccessToken
	ts.expires = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return ts.token, nil
}
