package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"render-ai/backend/internal/jobs"
	renderpkg "render-ai/backend/internal/render"
	"render-ai/backend/internal/store"
)

// enableAuthAndGrant flips authEnabled on for a Server built by
// setupRenderTest and grants ownerID exactly units credits (in the store's
// integer resolution) so a test can then exercise the exact-balance edge.
func enableAuthAndGrant(t *testing.T, s *Server, repo *store.MemoryStore, ownerID string, units int64) {
	t.Helper()
	s.cfg.Auth.ClerkSecretKey = "sk_test_dummy"
	if units == 0 {
		return
	}
	if _, err := repo.AdjustCredits(ownerID, units, store.CreditLedgerEntry{Reason: store.CreditReasonGrant}); err != nil {
		t.Fatalf("granting credits: %v", err)
	}
}

// TestStartRenderChargesCreditsAndReportsThem covers the happy charging
// path: the exact amount (unitsPerVariation * variations) is debited before
// the job is created, the response reports it as creditsCharged, and a fully
// successful job leaves the balance charged (no refund).
func TestStartRenderChargesCreditsAndReportsThem(t *testing.T) {
	s, repo, p, v, queue := setupRenderTest(t, alwaysSucceeds(t))
	perVar := unitsPerVariation(store.ModelPro, store.Resolution2K)
	enableAuthAndGrant(t, s, repo, p.OwnerID, perVar*2)

	resp, status := postStartRender(t, s, p.ID, v.ID, renderRequestBody{Model: "pro", Resolution: "2K", Variations: 2})
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	if resp.CreditsCharged != unitsToCredits(perVar*2) {
		t.Fatalf("CreditsCharged = %v, want %v", resp.CreditsCharged, unitsToCredits(perVar*2))
	}
	if units, err := repo.GetCredits(p.OwnerID); err != nil || units != 0 {
		t.Fatalf("balance right after charging = %d, %v, want 0", units, err)
	}

	queue.Wait()

	if units, err := repo.GetCredits(p.OwnerID); err != nil || units != 0 {
		t.Fatalf("balance after a fully successful job = %d, %v, want still 0 (no refund on success)", units, err)
	}
}

// TestStartRenderInsufficientCreditsIs402 covers the 402 path: with a zero
// balance the render is rejected before anything is created, and the balance
// (and ledger) are left untouched.
func TestStartRenderInsufficientCreditsIs402(t *testing.T) {
	s, repo, p, v, _ := setupRenderTest(t, alwaysSucceeds(t))
	enableAuthAndGrant(t, s, repo, p.OwnerID, 0)

	req := renderRequestBody{Model: "pro", Resolution: "2K", Variations: 1}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshaling request: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	r.SetPathValue("pid", p.ID)
	r.SetPathValue("vid", v.ID)

	handlerErr := s.startRender(httptest.NewRecorder(), r)
	if handlerErr == nil {
		t.Fatal("want an error, got nil")
	}
	var he *httpError
	if !errors.As(handlerErr, &he) {
		t.Fatalf("want *httpError, got %T (%v)", handlerErr, handlerErr)
	}
	if he.status != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", he.status)
	}
	if he.code != "insufficient_credits" {
		t.Fatalf("code = %q, want insufficient_credits", he.code)
	}

	if units, err := repo.GetCredits(p.OwnerID); err != nil || units != 0 {
		t.Fatalf("balance after a rejected render = %d, %v, want 0", units, err)
	}
	ledger, err := repo.ListCreditLedger(p.OwnerID, 10)
	if err != nil {
		t.Fatalf("ListCreditLedger: %v", err)
	}
	if len(ledger) != 0 {
		t.Fatalf("ledger after a rejected render = %+v, want empty", ledger)
	}
}

// TestStartRenderSkipsCreditsWhenAuthDisabled covers the dev-bypass: with
// CLERK_SECRET_KEY unset (the default in setupRenderTest), a render with a
// zero balance still succeeds, and nothing is ever charged.
func TestStartRenderSkipsCreditsWhenAuthDisabled(t *testing.T) {
	s, repo, p, v, _ := setupRenderTest(t, alwaysSucceeds(t))

	_, status := postStartRender(t, s, p.ID, v.ID, renderRequestBody{Model: "pro", Resolution: "2K", Variations: 1})
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (credits must be skipped when auth is disabled)", status)
	}
	if units, err := repo.GetCredits(p.OwnerID); err != nil || units != 0 {
		t.Fatalf("balance = %d, %v, want unchanged 0", units, err)
	}
}

// TestFailedVariationRefundsExactlyOnce covers the refund-once contract end
// to end: one variation of a two-variation job fails, its charge is refunded
// exactly once, and a redelivered/repeated failure of the same variation
// (via RunRenderVariation, and directly via failVariation) never refunds it
// again.
func TestFailedVariationRefundsExactlyOnce(t *testing.T) {
	renderer := &fakeRenderer{fn: func(idx int) (renderpkg.RenderResult, error) {
		if idx == 1 {
			return renderpkg.RenderResult{}, errors.New("simulated model failure")
		}
		return renderpkg.RenderResult{ImageData: testPNGBytes, MIMEType: "image/png"}, nil
	}}
	s, repo, p, v, queue := setupRenderTest(t, renderer)
	perVar := unitsPerVariation(store.ModelPro, store.Resolution2K)
	enableAuthAndGrant(t, s, repo, p.OwnerID, perVar*2)

	resp, status := postStartRender(t, s, p.ID, v.ID, renderRequestBody{Model: "pro", Resolution: "2K", Variations: 2})
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	queue.Wait()

	units, err := repo.GetCredits(p.OwnerID)
	if err != nil {
		t.Fatalf("GetCredits: %v", err)
	}
	if units != perVar {
		t.Fatalf("balance after one of two variations failed = %d units, want %d (charged for 2, refunded 1)", units, perVar)
	}

	// A redelivered task for the already-failed variation must be a no-op
	// (RunRenderVariation's own claim check already prevents this from
	// calling failVariation a second time - see TestFailVariationRefundsOnce
	// below for the belt-and-braces guarantee on failVariation itself).
	s.RunRenderVariation(context.Background(), jobs.Task{ProjectID: p.ID, JobID: resp.ID, Variation: 1})
	if units, err := repo.GetCredits(p.OwnerID); err != nil || units != perVar {
		t.Fatalf("balance after a redelivered failure = %d, %v, want still %d", units, err, perVar)
	}

	ledger, err := repo.ListCreditLedger(p.OwnerID, 10)
	if err != nil {
		t.Fatalf("ListCreditLedger: %v", err)
	}
	refunds := 0
	for _, e := range ledger {
		if e.Reason == store.CreditReasonRefund {
			refunds++
		}
	}
	if refunds != 1 {
		t.Fatalf("refund ledger entries = %d, want exactly 1", refunds)
	}
}

// TestFailVariationRefundsOnce is the belt-and-braces case the doc comment on
// failVariation promises: even if it's somehow invoked twice for the same
// variation (a second failure observation, not just a redelivered task), the
// Refunded flag - claimed inside the same UpdateRenderJob call as the
// failed-status write - stops a second refund.
func TestFailVariationRefundsOnce(t *testing.T) {
	s, repo, p, v, _ := setupRenderTest(t, alwaysSucceeds(t))
	perVar := unitsPerVariation(store.ModelPro, store.Resolution2K)
	enableAuthAndGrant(t, s, repo, p.OwnerID, perVar)

	job := &store.RenderJob{
		ID:     "job-1",
		ViewID: v.ID,
		Request: store.RenderJobRequest{
			Model: store.ModelPro, Resolution: store.Resolution2K, Variations: 1,
		},
		Variations:               []store.RenderJobVariation{{Status: store.RenderJobRunning}},
		ChargedUnitsPerVariation: perVar,
	}
	if _, err := repo.CreateRenderJob(p.ID, job); err != nil {
		t.Fatalf("CreateRenderJob: %v", err)
	}

	s.failVariation(p.ID, "job-1", 0, "boom")
	s.failVariation(p.ID, "job-1", 0, "boom again")

	units, err := repo.GetCredits(p.OwnerID)
	if err != nil {
		t.Fatalf("GetCredits: %v", err)
	}
	// The account started at perVar (the grant above - this test never
	// actually debits it, only exercises the refund side), so exactly one
	// refund lands at 2*perVar; a second refund would show up as 3*perVar.
	if units != 2*perVar {
		t.Fatalf("balance after failVariation called twice = %d units, want %d (refunded exactly once)", units, 2*perVar)
	}
}

// failingQueue always fails Enqueue, so startRender's "mark remaining
// variations failed and refund them" path can be exercised deterministically.
type failingQueue struct{}

func (failingQueue) Enqueue(context.Context, jobs.Task) error {
	return errors.New("simulated enqueue failure")
}

// TestStartRenderRefundsOnEnqueueFailure covers the "nothing was ever queued"
// case: when Enqueue itself fails, startRender reports 502 and refunds the
// full charge.
func TestStartRenderRefundsOnEnqueueFailure(t *testing.T) {
	s, repo, p, v, _ := setupRenderTest(t, alwaysSucceeds(t))
	perVar := unitsPerVariation(store.ModelPro, store.Resolution2K)
	grant := perVar * 2
	enableAuthAndGrant(t, s, repo, p.OwnerID, grant)
	s.queue = failingQueue{}

	_, status := postStartRender(t, s, p.ID, v.ID, renderRequestBody{Model: "pro", Resolution: "2K", Variations: 2})
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", status)
	}
	units, err := repo.GetCredits(p.OwnerID)
	if err != nil {
		t.Fatalf("GetCredits: %v", err)
	}
	if units != grant {
		t.Fatalf("balance after an enqueue failure = %d units, want %d (fully refunded)", units, grant)
	}
}
