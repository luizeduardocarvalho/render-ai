// Admin routes (requireAdmin - see roles.go): listing users from Clerk merged
// with their credit balance from the store, and granting/correcting credits.
// See API_CONTRACT.md's admin section.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/user"

	"render-ai/backend/internal/store"
)

// clerkUser is the subset of a Clerk user this package needs for the admin
// user list - decoupled from clerk.User so listClerkUsersFunc is easy to
// fake in tests (see NewServer's default, listClerkUsersViaAPI).
type clerkUser struct {
	ID           string
	Email        string
	FirstName    string
	LastName     string
	ImageURL     string
	Role         string
	CreatedAt    time.Time
	LastSignInAt *time.Time
}

// listClerkUsersFunc lists Clerk users matching query (their name/email
// search - "" means no filter), paginated by limit/offset, returning the
// page plus the total count matching query. Behind a Server func field (like
// oidcValidator) so tests can inject a fake and never need a real Clerk
// account or network access.
type listClerkUsersFunc func(ctx context.Context, query string, limit, offset int) ([]clerkUser, int64, error)

// listClerkUsersViaAPI is the default listClerkUsersFunc, calling the real
// Clerk Backend API (clerk-sdk-go v2's user.Client.List).
func listClerkUsersViaAPI(ctx context.Context, query string, limit, offset int) ([]clerkUser, int64, error) {
	client := user.NewClient(&clerk.ClientConfig{})
	limit64, offset64 := int64(limit), int64(offset)
	params := &user.ListParams{
		ListParams: clerk.ListParams{Limit: &limit64, Offset: &offset64},
	}
	if query != "" {
		params.Query = &query
	}
	list, err := client.List(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	out := make([]clerkUser, len(list.Users))
	for i, u := range list.Users {
		out[i] = clerkUserFrom(u)
	}
	return out, list.TotalCount, nil
}

// clerkUserFrom converts a clerk.User into our clerkUser, reading its role
// from publicMetadata exactly like userRole does (see roles.go) - listing
// users doesn't go through the per-user roleCache, since it already fetched
// every user's full record in one call.
func clerkUserFrom(u *clerk.User) clerkUser {
	out := clerkUser{
		ID:        u.ID,
		FirstName: derefStr(u.FirstName),
		LastName:  derefStr(u.LastName),
		ImageURL:  derefStr(u.ImageURL),
		CreatedAt: time.UnixMilli(u.CreatedAt).UTC(),
	}
	if u.PrimaryEmailAddressID != nil {
		for _, e := range u.EmailAddresses {
			if e.ID == *u.PrimaryEmailAddressID {
				out.Email = e.EmailAddress
				break
			}
		}
	}
	if out.Email == "" && len(u.EmailAddresses) > 0 {
		out.Email = u.EmailAddresses[0].EmailAddress
	}
	if len(u.PublicMetadata) > 0 {
		var meta publicMetadata
		if err := json.Unmarshal(u.PublicMetadata, &meta); err == nil {
			out.Role = meta.Role
		}
	}
	if u.LastSignInAt != nil {
		t := time.UnixMilli(*u.LastSignInAt).UTC()
		out.LastSignInAt = &t
	}
	return out
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// adminUser is one row of GET /api/admin/users - see API_CONTRACT.md's
// AdminUser.
type adminUser struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	FirstName    string     `json:"firstName"`
	LastName     string     `json:"lastName"`
	ImageURL     string     `json:"imageUrl"`
	Role         string     `json:"role"`
	Credits      float64    `json:"credits"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastSignInAt *time.Time `json:"lastSignInAt,omitempty"`
}

type adminUsersResponse struct {
	Users      []adminUser `json:"users"`
	TotalCount int64       `json:"totalCount"`
}

// defaultAdminUsersLimit/maxAdminUsersLimit bound GET /api/admin/users'
// ?limit= - see API_CONTRACT.md.
const (
	defaultAdminUsersLimit = 50
	maxAdminUsersLimit     = 200
)

// adminListUsers is GET /api/admin/users?query=&limit=&offset= - Clerk users
// merged with their credit balance from the store.
func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	query := q.Get("query")

	limit := defaultAdminUsersLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return badRequest("limit must be a positive integer")
		}
		if n > maxAdminUsersLimit {
			n = maxAdminUsersLimit
		}
		limit = n
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return badRequest("offset must be a non-negative integer")
		}
		offset = n
	}

	users, total, err := s.listClerkUsers(r.Context(), query, limit, offset)
	if err != nil {
		return badGateway("listing users: %v", err)
	}

	out := make([]adminUser, len(users))
	for i, u := range users {
		units, err := s.repo.GetCredits(u.ID)
		if err != nil {
			return internalErr("loading credits for %s: %v", u.ID, err)
		}
		out[i] = adminUser{
			ID: u.ID, Email: u.Email, FirstName: u.FirstName, LastName: u.LastName,
			ImageURL: u.ImageURL, Role: u.Role, Credits: unitsToCredits(units),
			CreatedAt: u.CreatedAt, LastSignInAt: u.LastSignInAt,
		}
	}
	writeJSON(w, http.StatusOK, adminUsersResponse{Users: out, TotalCount: total})
	return nil
}

// grantCreditsRequest is the JSON body of POST /api/admin/users/{uid}/credits.
type grantCreditsRequest struct {
	Amount float64 `json:"amount"`
	Note   string  `json:"note"`
}

// adminGrantCredits is POST /api/admin/users/{uid}/credits: a signed credits
// delta (grant or correction), non-zero, a multiple of 0.25, bounded by
// maxGrantCredits, that may not take the balance below zero - see
// API_CONTRACT.md.
func (s *Server) adminGrantCredits(w http.ResponseWriter, r *http.Request) error {
	uid := r.PathValue("uid")
	var req grantCreditsRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.Amount == 0 {
		return badRequest("amount must be non-zero")
	}
	if math.Abs(req.Amount) > maxGrantCredits {
		return badRequest("amount must be at most %d credits", maxGrantCredits)
	}
	units, err := creditsToUnits(req.Amount)
	if err != nil {
		return badRequest("amount %v", err)
	}

	actorID, _ := userIDFromContext(r.Context())
	balanceUnits, err := s.repo.AdjustCredits(uid, units, store.CreditLedgerEntry{
		Reason:  store.CreditReasonGrant,
		ActorID: actorID,
		Note:    req.Note,
	})
	if err != nil {
		if errors.Is(err, store.ErrInsufficientCredits) {
			return badRequest("amount would take the balance below zero")
		}
		return internalErr("granting credits: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]float64{"credits": unitsToCredits(balanceUnits)})
	return nil
}

// adminGetUserCredits is GET /api/admin/users/{uid}/credits: the same shape
// as GET /api/me/credits, for an admin-chosen user.
func (s *Server) adminGetUserCredits(w http.ResponseWriter, r *http.Request) error {
	uid := r.PathValue("uid")
	return s.writeCreditsResponse(w, uid)
}
