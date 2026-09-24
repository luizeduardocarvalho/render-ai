package api

import (
	"context"

	"google.golang.org/api/idtoken"
)

// oidcClaims is the subset of a validated Google OIDC token's claims the
// worker route cares about.
type oidcClaims struct {
	Email         string
	EmailVerified bool
}

// OIDCValidator validates a bearer token against audience and returns its
// claims, or an error if the token is missing, expired, or otherwise
// invalid. It is a field on Server (not a package-level function call) so
// tests can inject a fake and never need real Google credentials or network
// access - see api.go's Server.oidcValidator and render.go's
// verifyWorkerRequest.
type OIDCValidator func(ctx context.Context, token, audience string) (oidcClaims, error)

// verifyGoogleOIDCToken is the production OIDCValidator: it validates token
// as a real Google-signed ID token via google.golang.org/api/idtoken, which
// checks the signature against Google's published JWKS, the audience, and
// expiry.
func verifyGoogleOIDCToken(ctx context.Context, token, audience string) (oidcClaims, error) {
	payload, err := idtoken.Validate(ctx, token, audience)
	if err != nil {
		return oidcClaims{}, err
	}
	email, _ := payload.Claims["email"].(string)
	verified, _ := payload.Claims["email_verified"].(bool)
	return oidcClaims{Email: email, EmailVerified: verified}, nil
}
