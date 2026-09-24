package api

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
)

// handlerFunc is the error-returning handler shape used throughout this
// package; s.handle adapts it to an http.HandlerFunc, and the requireAuth /
// requireAdmin / requireOwner middlewares wrap it.
type handlerFunc = func(w http.ResponseWriter, r *http.Request) error

// ctxKey is a private type for context keys defined in this package, so they
// can't collide with keys set by other packages.
type ctxKey int

const (
	userIDCtxKey ctxKey = iota
	roleCtxKey
)

// userIDFromContext returns the Clerk user id placed in the request context
// by requireAuth or requireAdmin, if any.
func userIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDCtxKey).(string)
	return id, ok
}

// roleFromContext returns the Clerk publicMetadata role placed in the
// request context by requireAdmin, if any.
func roleFromContext(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(roleCtxKey).(string)
	return role, ok
}

// initAuth configures the Clerk SDK with the secret key from cfg, if set.
// When the key is empty, auth is disabled for this PoC deployment: a single
// prominent warning is logged at startup and requireAuth becomes a no-op, so
// local dev without Clerk credentials keeps working. Mirrors how the server
// degrades when Vertex AI credentials are absent (see cmd/server/main.go).
func initAuth(secretKey string) {
	if secretKey == "" {
		log.Println("server: WARNING - auth disabled: CLERK_SECRET_KEY not set")
		return
	}
	clerk.SetKey(secretKey)
}

// verifiedUserID extracts the bearer token from r and verifies it against
// Clerk's JWKS (networkless: the signature is checked locally against
// Clerk's public key, no call to the Clerk Sessions API), returning the
// verified user id. requireAuth and requireAdmin both call this so the
// token-verification logic lives in exactly one place.
func (s *Server) verifiedUserID(r *http.Request) (string, error) {
	token := bearerToken(r)
	if token == "" {
		return "", unauthorized("missing bearer token")
	}

	claims, err := jwt.Verify(r.Context(), &jwt.VerifyParams{Token: token})
	if err != nil {
		return "", unauthorized("invalid or expired token: %v", err)
	}

	return claims.Subject, nil
}

// requireAuth wraps fn with Clerk session-token verification (see
// verifiedUserID) and stores the verified user id in the request context. On
// any failure it responds 401 with the standard {"error": "..."} shape
// instead of calling fn.
//
// If CLERK_SECRET_KEY is unset (s.cfg.Auth.ClerkSecretKey == ""), verification
// is skipped entirely and fn is called as-is - see initAuth.
func (s *Server) requireAuth(fn func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		if s.cfg.Auth.ClerkSecretKey == "" {
			return fn(w, r)
		}

		userID, err := s.verifiedUserID(r)
		if err != nil {
			return err
		}

		ctx := context.WithValue(r.Context(), userIDCtxKey, userID)
		return fn(w, r.WithContext(ctx))
	}
}

// requireOwner gates a project-scoped handler on the verified caller owning
// the project named by the {pid} path value. It must be composed inside
// requireAuth, which puts the verified user id in the request context - any
// signed-in user may reach it, not just admins (see Router's doc comment:
// only /api/admin/* stays admin-gated).
//
// A non-owner (or a missing project) gets a 404, never a 403 - so a user
// cannot probe which project ids exist by owner. When CLERK_SECRET_KEY is
// unset (auth disabled for local dev), the check is skipped: there is no
// verified user and every project is implicitly owned by "".
func (s *Server) requireOwner(fn handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		if s.cfg.Auth.ClerkSecretKey == "" {
			return fn(w, r)
		}
		pid := r.PathValue("pid")
		userID, ok := userIDFromContext(r.Context())
		if !ok {
			// requireAdmin should have populated this; treat its absence as deny.
			return notFoundErr("project %s not found", pid)
		}
		ownerID, err := s.repo.ProjectOwner(pid)
		if err != nil {
			return mapStoreErr(err, "project %s not found", pid)
		}
		if ownerID != userID {
			return notFoundErr("project %s not found", pid)
		}
		return fn(w, r)
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header, returning "" if the header is missing or malformed.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, prefix))
}
