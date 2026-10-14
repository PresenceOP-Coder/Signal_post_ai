package model

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

var (
	ErrEmptyKey             = errors.New("fact key cannot be empty")
	ErrEmptyValue           = errors.New("fact value cannot be nil")
	ErrEmptySourceURL       = errors.New("fact sourceURL cannot be empty")
	ErrEmptyEvidence        = errors.New("fact evidence cannot be empty")
	ErrZeroRetrievedAt      = errors.New("fact retrievedAt cannot be zero")
	ErrInvalidOrigin        = errors.New("invalid origin")
	ErrFinancialOrigin      = errors.New("financial facts must have origin 'accounts'")
	ErrLLMClaimNumericValue = errors.New("llm_claim origin cannot produce numeric values")
	ErrLLMClaimFinancialKey = errors.New("llm_claim origin cannot produce financial keys")
)

// Well-known financial keys that must strictly come from official annual accounts.
var financialKeys = map[string]bool{
	"revenue":          true,
	"operating_result": true,
	"net_result":       true,
	"total_assets":     true,
	"equity":           true,
	"fiscal_year":      true,
}

// IsFinancialKey returns true if the key represents a financial metric.
func IsFinancialKey(key string) bool {
	return financialKeys[strings.ToLower(strings.TrimSpace(key))]
}

// Fact represents an individual unit of verified information about a company.
type Fact struct {
	Key         string    `json:"key"`
	Value       any       `json:"value"`
	Unit        string    `json:"unit,omitempty"`
	Period      string    `json:"period,omitempty"`
	SourceURL   string    `json:"source_url"`
	RetrievedAt time.Time `json:"retrieved_at"`
	Origin      Origin    `json:"origin"`
	Evidence    string    `json:"evidence"`
}

// CompositeKey returns a unique identifier for diffing (e.g., "revenue:2023" or "legal_name").
func (f Fact) CompositeKey() string {
	if f.Period != "" {
		return fmt.Sprintf("%s:%s", f.Key, f.Period)
	}
	return f.Key
}

func isNumeric(val any) bool {
	if val == nil {
		return false
	}
	switch val.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	default:
		kind := reflect.TypeOf(val).Kind()
		return kind >= reflect.Int && kind <= reflect.Float64
	}
}

// NewFact builds and validates a new Fact adhering to strict invariants.
func NewFact(key string, val any, unit, period, sourceURL string, retrievedAt time.Time, origin Origin, evidence string) (Fact, error) {
	cleanKey := strings.TrimSpace(key)
	if cleanKey == "" {
		return Fact{}, ErrEmptyKey
	}
	if val == nil {
		return Fact{}, ErrEmptyValue
	}
	cleanSourceURL := strings.TrimSpace(sourceURL)
	if cleanSourceURL == "" {
		return Fact{}, ErrEmptySourceURL
	}
	cleanEvidence := strings.TrimSpace(evidence)
	if cleanEvidence == "" {
		return Fact{}, ErrEmptyEvidence
	}
	if retrievedAt.IsZero() {
		return Fact{}, ErrZeroRetrievedAt
	}
	if !origin.IsValid() {
		return Fact{}, ErrInvalidOrigin
	}

	// Financial invariant: financial keys can only originate from official accounts
	if IsFinancialKey(cleanKey) && origin != OriginAccounts {
		return Fact{}, fmt.Errorf("%w: key %q has origin %q", ErrFinancialOrigin, cleanKey, origin)
	}

	// LLM claim invariants
	if origin == OriginLLMClaim {
		if IsFinancialKey(cleanKey) {
			return Fact{}, fmt.Errorf("%w: key %q", ErrLLMClaimFinancialKey, cleanKey)
		}
		if isNumeric(val) {
			return Fact{}, fmt.Errorf("%w: key %q value %v", ErrLLMClaimNumericValue, cleanKey, val)
		}
	}

	return Fact{
		Key:         cleanKey,
		Value:       val,
		Unit:        strings.TrimSpace(unit),
		Period:      strings.TrimSpace(period),
		SourceURL:   cleanSourceURL,
		RetrievedAt: retrievedAt,
		Origin:      origin,
		Evidence:    cleanEvidence,
	}, nil
}
