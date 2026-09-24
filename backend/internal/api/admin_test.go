package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"render-ai/backend/internal/jobs"
	"render-ai/backend/internal/store"
)

// newAdminTestServer builds a Server with auth enabled (so requireAdmin's
// role check would run for real if it were exercised - these tests call the
// admin handlers directly, like auth_test.go does for requireOwner) and an
// in-memory store.
func newAdminTestServer() (*Server, *store.MemoryStore) {
	st := store.NewMemory()
	return NewServer(st, st, nil, nil, authedConfig(), "", jobs.NewInline()), st
}

// postGrantCredits calls adminGrantCredits directly (bypassing requireAdmin,
// same pattern as auth_test.go's call helper) as actorID, returning the
// resulting HTTP status and, on success, the new balance in credits.
func postGrantCredits(t *testing.T, s *Server, uid string, amount float64, note, actorID string) (int, float64) {
	t.Helper()
	r := jsonRequest(t, grantCreditsRequest{Amount: amount, Note: note})
	r.SetPathValue("uid", uid)
	if actorID != "" {
		r = r.WithContext(context.WithValue(r.Context(), userIDCtxKey, actorID))
	}
	rec := httptest.NewRecorder()
	if err := s.adminGrantCredits(rec, r); err != nil {
		status, _ := statusAndMessage(err)
		return status, 0
	}
	var resp map[string]float64
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding grant response: %v", err)
	}
	return rec.Code, resp["credits"]
}

// TestAdminGrantCreditsValidatesAmount covers the request-shape validation
// from API_CONTRACT.md: non-zero, a multiple of 0.25, |amount| <= 10000.
func TestAdminGrantCreditsValidatesAmount(t *testing.T) {
	s, _ := newAdminTestServer()
	cases := []struct {
		name   string
		amount float64
	}{
		{"zero", 0},
		{"not a multiple of 0.25", 1.1},
		{"too large positive", 10000.25},
		{"too large negative", -10000.25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := postGrantCredits(t, s, "user-a", tc.amount, "", "admin-1")
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", status)
			}
		})
	}
}

// TestAdminGrantCreditsAcceptsTheBoundary confirms exactly +-10000 (a
// multiple of 0.25) is allowed, not rejected as "too large".
func TestAdminGrantCreditsAcceptsTheBoundary(t *testing.T) {
	s, repo := newAdminTestServer()
	status, credits := postGrantCredits(t, s, "user-a", 10000, "", "admin-1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if credits != 10000 {
		t.Fatalf("credits = %v, want 10000", credits)
	}
	status, credits = postGrantCredits(t, s, "user-a", -10000, "", "admin-1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if credits != 0 {
		t.Fatalf("credits = %v, want 0", credits)
	}
	_ = repo
}

// TestAdminGrantCreditsRejectsNegativeBalance covers the "may not go below
// zero" rule: a correction larger than the current balance is a 400, and
// nothing is written.
func TestAdminGrantCreditsRejectsNegativeBalance(t *testing.T) {
	s, repo := newAdminTestServer()
	status, _ := postGrantCredits(t, s, "user-a", -1, "", "admin-1")
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	units, err := repo.GetCredits("user-a")
	if err != nil {
		t.Fatalf("GetCredits: %v", err)
	}
	if units != 0 {
		t.Fatalf("balance changed by a rejected grant: %d units", units)
	}
	ledger, err := repo.ListCreditLedger("user-a", 10)
	if err != nil {
		t.Fatalf("ListCreditLedger: %v", err)
	}
	if len(ledger) != 0 {
		t.Fatalf("a rejected grant left a ledger entry: %+v", ledger)
	}
}

// TestAdminGrantCreditsAppliesValidGrant covers the success path: the ledger
// entry records reason "grant", the acting admin's id, and the note, and the
// response/store balance agree.
func TestAdminGrantCreditsAppliesValidGrant(t *testing.T) {
	s, repo := newAdminTestServer()
	status, credits := postGrantCredits(t, s, "user-a", 5.25, "welcome bonus", "admin-1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if credits != 5.25 {
		t.Fatalf("credits = %v, want 5.25", credits)
	}
	units, err := repo.GetCredits("user-a")
	if err != nil {
		t.Fatalf("GetCredits: %v", err)
	}
	if units != 21 {
		t.Fatalf("units = %d, want 21 (5.25 credits * 4)", units)
	}
	ledger, err := repo.ListCreditLedger("user-a", 10)
	if err != nil {
		t.Fatalf("ListCreditLedger: %v", err)
	}
	if len(ledger) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(ledger))
	}
	e := ledger[0]
	if e.Reason != store.CreditReasonGrant || e.ActorID != "admin-1" || e.Note != "welcome bonus" {
		t.Fatalf("unexpected ledger entry: %+v", e)
	}
	if e.DeltaUnits != 21 || e.BalanceAfterUnits != 21 {
		t.Fatalf("unexpected ledger delta/balance: %+v", e)
	}
}

// TestAdminGrantCreditsAllowsNegativeCorrectionAboveZero covers a downward
// correction that stays at or above zero.
func TestAdminGrantCreditsAllowsNegativeCorrectionAboveZero(t *testing.T) {
	s, repo := newAdminTestServer()
	if _, err := repo.AdjustCredits("user-a", 40, store.CreditLedgerEntry{Reason: store.CreditReasonGrant}); err != nil {
		t.Fatalf("seeding balance: %v", err)
	}
	status, credits := postGrantCredits(t, s, "user-a", -2.5, "correction", "admin-1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if credits != 7.5 {
		t.Fatalf("credits = %v, want 7.5", credits)
	}
}

// TestAdminListUsersMergesCredits covers GET /api/admin/users: the fake
// listClerkUsers func's query/limit/offset are forwarded, and each returned
// user's credits come from the store, not Clerk.
func TestAdminListUsersMergesCredits(t *testing.T) {
	s, repo := newAdminTestServer()
	if _, err := repo.AdjustCredits("user-a", 8, store.CreditLedgerEntry{Reason: store.CreditReasonGrant}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	var gotQuery string
	var gotLimit, gotOffset int
	s.listClerkUsers = func(_ context.Context, query string, limit, offset int) ([]clerkUser, int64, error) {
		gotQuery, gotLimit, gotOffset = query, limit, offset
		return []clerkUser{{ID: "user-a", Email: "a@example.com", Role: "member"}}, 1, nil
	}

	r := httptest.NewRequest(http.MethodGet, "/api/admin/users?query=abc&limit=10&offset=5", nil)
	rec := httptest.NewRecorder()
	if err := s.adminListUsers(rec, r); err != nil {
		t.Fatalf("adminListUsers: %v", err)
	}
	if gotQuery != "abc" || gotLimit != 10 || gotOffset != 5 {
		t.Fatalf("query/limit/offset not forwarded: query=%q limit=%d offset=%d", gotQuery, gotLimit, gotOffset)
	}

	var resp adminUsersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.TotalCount != 1 || len(resp.Users) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Users[0].ID != "user-a" || resp.Users[0].Credits != 2 {
		t.Fatalf("unexpected user row: %+v, want credits=2", resp.Users[0])
	}
}

// TestAdminListUsersDefaultsLimit covers the default/clamped ?limit=.
func TestAdminListUsersDefaultsLimit(t *testing.T) {
	s, _ := newAdminTestServer()
	var gotLimit int
	s.listClerkUsers = func(_ context.Context, _ string, limit, _ int) ([]clerkUser, int64, error) {
		gotLimit = limit
		return nil, 0, nil
	}
	r := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	if err := s.adminListUsers(httptest.NewRecorder(), r); err != nil {
		t.Fatalf("adminListUsers: %v", err)
	}
	if gotLimit != defaultAdminUsersLimit {
		t.Fatalf("default limit = %d, want %d", gotLimit, defaultAdminUsersLimit)
	}
}

// TestAdminGetUserCredits covers GET /api/admin/users/{uid}/credits - same
// shape as GET /api/me/credits, for an admin-chosen user.
func TestAdminGetUserCredits(t *testing.T) {
	s, repo := newAdminTestServer()
	if _, err := repo.AdjustCredits("user-a", 12, store.CreditLedgerEntry{Reason: store.CreditReasonGrant}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("uid", "user-a")
	rec := httptest.NewRecorder()
	if err := s.adminGetUserCredits(rec, r); err != nil {
		t.Fatalf("adminGetUserCredits: %v", err)
	}
	var resp meCreditsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.Credits != 3 {
		t.Fatalf("credits = %v, want 3", resp.Credits)
	}
	if len(resp.Ledger) != 1 || resp.Ledger[0].Reason != store.CreditReasonGrant {
		t.Fatalf("unexpected ledger: %+v", resp.Ledger)
	}
}
