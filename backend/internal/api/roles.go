package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/user"
)

// roleCacheTTL bounds how long a looked-up role is trusted before we go back
// to Clerk. This keeps admin-gated routes from calling the Clerk API on
// every request, at the cost of role changes made in the Clerk dashboard (or
// via the Clerk API) taking up to roleCacheTTL to take effect here.
const roleCacheTTL = 60 * time.Second

// roleCache is a small concurrency-safe cache of Clerk user id -> role.
type roleCache struct {
	mu      sync.Mutex
	entries map[string]roleCacheEntry
}

type roleCacheEntry struct {
	role      string
	expiresAt time.Time
}

func newRoleCache() *roleCache {
	return &roleCache{entries: make(map[string]roleCacheEntry)}
}

func (c *roleCache) get(userID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[userID]
	if !ok || time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.role, true
}

func (c *roleCache) set(userID, role string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[userID] = roleCacheEntry{role: role, expiresAt: time.Now().Add(roleCacheTTL)}
}

// publicMetadata mirrors the one field we rely on in a Clerk user's
// publicMetadata for authorization. It is set out-of-band (Clerk dashboard
// or Clerk API), never by this backend and never by the client.
type publicMetadata struct {
	Role string `json:"role"`
}

// userRole returns the role recorded in userID's Clerk publicMetadata (""
// if absent), consulting the short-lived roleCache first and only calling
// the Clerk API on a miss. On a Clerk API error the error is returned
// unchanged; callers must treat that as "deny", never as "no role".
func (s *Server) userRole(ctx context.Context, userID string) (string, error) {
	if role, ok := s.roleCache.get(userID); ok {
		return role, nil
	}

	client := user.NewClient(&clerk.ClientConfig{})
	u, err := client.Get(ctx, userID)
	if err != nil {
		return "", err
	}

	var meta publicMetadata
	if len(u.PublicMetadata) > 0 {
		if err := json.Unmarshal(u.PublicMetadata, &meta); err != nil {
			return "", err
		}
	}

	s.roleCache.set(userID, meta.Role)
	return meta.Role, nil
}

// requireAdmin wraps fn with the same Clerk token verification as
// requireAuth (see verifiedUserID), plus a role check: the verified user's
// Clerk publicMetadata role must be exactly "admin", read fresh from Clerk
// (subject to roleCache) - never from anything the client sends. This is the
// default-deny gate for routes that spend real Vertex AI credits: any
// signed-in user without the admin role gets a 403, not a 200.
//
// If CLERK_SECRET_KEY is unset (s.cfg.Auth.ClerkSecretKey == ""), both token
// verification and the role check are skipped and fn is called as-is - see
// initAuth. This only happens in local dev without Clerk credentials.
func (s *Server) requireAdmin(fn func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		if s.cfg.Auth.ClerkSecretKey == "" {
			return fn(w, r)
		}

		userID, err := s.verifiedUserID(r)
		if err != nil {
			return err
		}

		role, err := s.userRole(r.Context(), userID)
		if err != nil {
			return forbidden("admin access required")
		}
		if role != "admin" {
			return forbidden("admin access required")
		}

		ctx := context.WithValue(r.Context(), userIDCtxKey, userID)
		ctx = context.WithValue(ctx, roleCtxKey, role)
		return fn(w, r.WithContext(ctx))
	}
}

// getMeResponse is the JSON body returned by GET /api/me.
type getMeResponse struct {
	UserID  string `json:"userId"`
	Role    string `json:"role"`
	IsAdmin bool   `json:"isAdmin"`
}

// getMe reports the caller's own Clerk user id and role, read server-side
// exactly like requireAdmin does, so the frontend can learn whether it's
// talking to an admin session without eating a 403 from an admin-gated
// route. Unlike the data/render routes, this only requires a signed-in user
// (see Router), not an admin one.
func (s *Server) getMe(w http.ResponseWriter, r *http.Request) error {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		// Only reached when CLERK_SECRET_KEY is unset, i.e. auth is disabled
		// for local dev (see initAuth): requireAuth calls straight through
		// without verifying a token, so there is no userID in context.
		// Report as admin so the dev frontend behaves like a real admin
		// session.
		writeJSON(w, http.StatusOK, getMeResponse{UserID: "", Role: "admin", IsAdmin: true})
		return nil
	}

	role, err := s.userRole(r.Context(), userID)
	if err != nil {
		return internalErr("looking up role: %v", err)
	}

	writeJSON(w, http.StatusOK, getMeResponse{UserID: userID, Role: role, IsAdmin: role == "admin"})
	return nil
}
