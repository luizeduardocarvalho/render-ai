package store

import "testing"

// TestMemoryStoreCreditsGrantChargeRefund covers the core AdjustCredits
// contract: a new account starts at 0, deltas apply atomically, an
// overdrawing delta is rejected and writes nothing, and the ledger records
// only the deltas that actually applied, newest first.
func TestMemoryStoreCreditsGrantChargeRefund(t *testing.T) {
	s := NewMemory()

	if units, err := s.GetCredits("user-a"); err != nil || units != 0 {
		t.Fatalf("GetCredits for a new account: units=%d err=%v, want 0, nil", units, err)
	}

	balance, err := s.AdjustCredits("user-a", 40, CreditLedgerEntry{Reason: CreditReasonGrant, Note: "welcome"})
	if err != nil {
		t.Fatalf("AdjustCredits (grant): %v", err)
	}
	if balance != 40 {
		t.Fatalf("balance after grant = %d, want 40", balance)
	}

	balance, err = s.AdjustCredits("user-a", -4, CreditLedgerEntry{Reason: CreditReasonRender})
	if err != nil {
		t.Fatalf("AdjustCredits (charge): %v", err)
	}
	if balance != 36 {
		t.Fatalf("balance after charge = %d, want 36", balance)
	}

	if _, err := s.AdjustCredits("user-a", -1000, CreditLedgerEntry{Reason: CreditReasonRender}); err != ErrInsufficientCredits {
		t.Fatalf("overdrawing charge: want ErrInsufficientCredits, got %v", err)
	}
	if units, _ := s.GetCredits("user-a"); units != 36 {
		t.Fatalf("balance after a rejected charge = %d, want unchanged 36", units)
	}

	ledger, err := s.ListCreditLedger("user-a", 10)
	if err != nil {
		t.Fatalf("ListCreditLedger: %v", err)
	}
	if len(ledger) != 2 {
		t.Fatalf("ledger entries = %d, want 2 (the rejected charge must not appear)", len(ledger))
	}
	if ledger[0].Reason != CreditReasonRender || ledger[0].DeltaUnits != -4 || ledger[0].BalanceAfterUnits != 36 {
		t.Fatalf("unexpected newest entry: %+v", ledger[0])
	}
	if ledger[1].Reason != CreditReasonGrant || ledger[1].DeltaUnits != 40 || ledger[1].BalanceAfterUnits != 40 {
		t.Fatalf("unexpected oldest entry: %+v", ledger[1])
	}
	if ledger[0].ID == "" || ledger[0].CreatedAt.IsZero() {
		t.Fatalf("ledger entry missing ID/CreatedAt: %+v", ledger[0])
	}

	limited, err := s.ListCreditLedger("user-a", 1)
	if err != nil {
		t.Fatalf("ListCreditLedger (limit 1): %v", err)
	}
	if len(limited) != 1 || limited[0].DeltaUnits != -4 {
		t.Fatalf("limited ledger = %+v, want just the newest entry", limited)
	}

	// A different user's account is untouched.
	if units, err := s.GetCredits("user-b"); err != nil || units != 0 {
		t.Fatalf("GetCredits for a different user: units=%d err=%v, want 0, nil", units, err)
	}
}

// TestMemoryStoreAdjustCreditsRejectsExactlyZeroingBelow confirms the
// boundary: a delta that leaves the balance at exactly 0 is fine, one that
// would take it to -1 is not.
func TestMemoryStoreAdjustCreditsBoundary(t *testing.T) {
	s := NewMemory()
	if _, err := s.AdjustCredits("user-a", 4, CreditLedgerEntry{Reason: CreditReasonGrant}); err != nil {
		t.Fatalf("AdjustCredits (grant): %v", err)
	}
	if balance, err := s.AdjustCredits("user-a", -4, CreditLedgerEntry{Reason: CreditReasonRender}); err != nil || balance != 0 {
		t.Fatalf("AdjustCredits down to exactly 0: balance=%d err=%v, want 0, nil", balance, err)
	}
	if _, err := s.AdjustCredits("user-a", -1, CreditLedgerEntry{Reason: CreditReasonRender}); err != ErrInsufficientCredits {
		t.Fatalf("AdjustCredits below 0: want ErrInsufficientCredits, got %v", err)
	}
}
