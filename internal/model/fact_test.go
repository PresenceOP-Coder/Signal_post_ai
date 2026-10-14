package model

import (
	"errors"
	"testing"
	"time"
)

func TestNewFactValidation(t *testing.T) {
	now := time.Now().UTC()

	// 1. Valid registry fact
	f, err := NewFact("legal_name", "EQUINOR ASA", "", "", "https://data.brreg.no/enhetsregisteret/api/enheter/923609016", now, OriginRegistry, "Enhetsregisteret official legal name")
	if err != nil {
		t.Fatalf("Valid fact creation failed: %v", err)
	}
	if f.CompositeKey() != "legal_name" {
		t.Errorf("CompositeKey = %q; want %q", f.CompositeKey(), "legal_name")
	}

	// 2. Valid financial fact
	fFin, err := NewFact("revenue", 1250000000, "NOK", "2023", "https://data.brreg.no/regnskapsregisteret/regnskap/923609016", now, OriginAccounts, "Regnskapsregisteret official 2023 accounts")
	if err != nil {
		t.Fatalf("Valid financial fact creation failed: %v", err)
	}
	if fFin.CompositeKey() != "revenue:2023" {
		t.Errorf("CompositeKey = %q; want %q", fFin.CompositeKey(), "revenue:2023")
	}

	// 3. Invariant: Financial keys must have OriginAccounts
	_, err = NewFact("revenue", 1250000000, "NOK", "2023", "https://example.com", now, OriginRegistry, "Some evidence")
	if !errors.Is(err, ErrFinancialOrigin) {
		t.Errorf("Expected ErrFinancialOrigin when creating financial fact with OriginRegistry, got: %v", err)
	}

	// 4. Invariant: Empty fields
	_, err = NewFact("", "val", "", "", "https://example.com", now, OriginRegistry, "evidence")
	if !errors.Is(err, ErrEmptyKey) {
		t.Errorf("Expected ErrEmptyKey, got: %v", err)
	}

	_, err = NewFact("key", nil, "", "", "https://example.com", now, OriginRegistry, "evidence")
	if !errors.Is(err, ErrEmptyValue) {
		t.Errorf("Expected ErrEmptyValue, got: %v", err)
	}

	_, err = NewFact("key", "val", "", "", "", now, OriginRegistry, "evidence")
	if !errors.Is(err, ErrEmptySourceURL) {
		t.Errorf("Expected ErrEmptySourceURL, got: %v", err)
	}

	_, err = NewFact("key", "val", "", "", "https://example.com", now, OriginRegistry, "")
	if !errors.Is(err, ErrEmptyEvidence) {
		t.Errorf("Expected ErrEmptyEvidence, got: %v", err)
	}

	_, err = NewFact("key", "val", "", "", "https://example.com", time.Time{}, OriginRegistry, "evidence")
	if !errors.Is(err, ErrZeroRetrievedAt) {
		t.Errorf("Expected ErrZeroRetrievedAt, got: %v", err)
	}

	// 5. Invariant: LLM claims cannot produce numbers or financial keys
	_, err = NewFact("description", 12345, "", "", "https://example.com", now, OriginLLMClaim, "evidence")
	if !errors.Is(err, ErrLLMClaimNumericValue) {
		t.Errorf("Expected ErrLLMClaimNumericValue, got: %v", err)
	}

	_, err = NewFact("revenue", "not-a-number-string", "", "2023", "https://example.com", now, OriginLLMClaim, "evidence")
	if err == nil {
		t.Errorf("Expected error when LLM claim targets financial key, got nil")
	}
}
