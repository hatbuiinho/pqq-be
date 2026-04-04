package auth

import "context"

type contextKey string

const claimsContextKey contextKey = "auth_claims"

func WithClaims(ctx context.Context, claims *Claims) context.Context {
	if claims == nil {
		return ctx
	}
	return context.WithValue(ctx, claimsContextKey, claims)
}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	value := ctx.Value(claimsContextKey)
	if value == nil {
		return nil, false
	}
	claims, ok := value.(*Claims)
	return claims, ok
}
