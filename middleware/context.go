package middleware

import "context"

func contextWithNonce(ctx context.Context, nonce string) context.Context {
	return context.WithValue(ctx, nonceKey, nonce)
}
