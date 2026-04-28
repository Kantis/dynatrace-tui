package grail

import (
	"context"
	"net/url"
)

// Cancel asks Grail to abort a running query. Safe to call after the query has finished.
func (c *Client) Cancel(ctx context.Context, requestToken string) error {
	q := url.Values{}
	q.Set("request-token", requestToken)
	_, err := c.do(ctx, "POST", "/query:cancel", q, nil)
	return err
}
