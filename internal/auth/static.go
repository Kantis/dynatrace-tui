package auth

import "context"

// Static is a TokenProvider that returns a pre-issued bearer token unchanged.
// Used for Dynatrace Platform Tokens (prefix dt0s16) which are presented
// directly as `Authorization: Bearer <token>` — no SSO exchange required.
type Static string

func (s Static) Token(_ context.Context) (string, error) { return string(s), nil }
