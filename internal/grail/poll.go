package grail

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// Poll fetches the current state of a running query. requestTimeoutSec controls
// long-poll behavior on the server side (clamped to ~60 by Grail).
func (c *Client) Poll(ctx context.Context, requestToken string, requestTimeoutSec int) (*Response, error) {
	q := url.Values{}
	q.Set("request-token", requestToken)
	if requestTimeoutSec > 0 {
		q.Set("request-timeout", strconv.Itoa(requestTimeoutSec))
	}
	return c.do(ctx, "GET", "/query:poll", q, nil)
}

// PollUntilDone polls repeatedly until the query reaches a terminal state or ctx is done.
// Uses long-polling on each request, so backoff between requests is minimal.
func (c *Client) PollUntilDone(ctx context.Context, requestToken string) (*Response, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := c.Poll(ctx, requestToken, 30)
		if err != nil {
			return nil, err
		}
		switch resp.State {
		case StateSucceeded, StateFailed, StateCancelled:
			return resp, nil
		case StateRunning:
			// Server long-poll already throttled us; tiny pause is enough as a safety net.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		default:
			return nil, fmt.Errorf("unexpected state %q", resp.State)
		}
	}
}
