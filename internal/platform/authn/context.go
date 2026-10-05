package authn

import "context"

type jtiKey struct{}

func withJTI(ctx context.Context, jti string) context.Context {
	return context.WithValue(ctx, jtiKey{}, jti)
}

// JTIFrom returns the current access token's id — logout needs it to
// deny-list exactly the presented token.
func JTIFrom(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(jtiKey{}).(string)
	return v, ok
}
