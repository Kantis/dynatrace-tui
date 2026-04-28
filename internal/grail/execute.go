package grail

import "context"

type ExecuteRequest struct {
	Query                     string `json:"query"`
	RequestTimeoutMillis      int    `json:"requestTimeoutMilliseconds,omitempty"`
	FetchTimeoutSeconds       int    `json:"fetchTimeoutSeconds,omitempty"`
	DefaultTimeframeStart     string `json:"defaultTimeframeStart,omitempty"`
	DefaultTimeframeEnd       string `json:"defaultTimeframeEnd,omitempty"`
	Timezone                  string `json:"timezone,omitempty"`
	Locale                    string `json:"locale,omitempty"`
}

// Execute starts a query. If Grail returns a final state synchronously, Result is populated.
// Otherwise the caller should poll using RequestToken.
func (c *Client) Execute(ctx context.Context, req ExecuteRequest) (*Response, error) {
	return c.do(ctx, "POST", "/query:execute", nil, req)
}
