package middleware

import "context"

// contextWithNonce returns a copy of ctx with the CSP nonce value stored
// under nonceKey. Kept in a separate file to avoid import cycles.
func contextWithNonce(ctx context.Context, nonce string) context.Context {
	return context.WithValue(ctx, nonceKey, nonce)
}
