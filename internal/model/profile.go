package model

import (
	"time"
)

// Profile represents a company profile containing verified facts, diff history, and explanations.
type Profile struct {
	Orgnr          string          `json:"orgnr"`
	LegalName      string          `json:"legal_name"`
	Status         string          `json:"status"`
	Facts          map[string]Fact `json:"facts"`
	History        []Change        `json:"history"`
	Summary        string          `json:"summary"`
	ChangesSummary []string        `json:"changes_summary"`
	Gaps           []string        `json:"gaps"`
	LastVerified   time.Time       `json:"last_verified"`
	LastChanged    time.Time       `json:"last_changed"`
	CreatedAt      time.Time       `json:"created_at"`
	Errors         []string        `json:"errors,omitempty"`
}

// NewProfile initializes an empty company profile for a given orgnr.
func NewProfile(orgnr string) *Profile {
	now := time.Now().UTC()
	return &Profile{
		Orgnr:          orgnr,
		Facts:          make(map[string]Fact),
		History:        make([]Change, 0),
		ChangesSummary: make([]string, 0),
		Gaps:           make([]string, 0),
		LastVerified:   now,
		LastChanged:    now,
		CreatedAt:      now,
	}
}

// AddFact inserts or updates a fact into the profile using its CompositeKey.
func (p *Profile) AddFact(f Fact) {
	if p.Facts == nil {
		p.Facts = make(map[string]Fact)
	}
	p.Facts[f.CompositeKey()] = f

	if f.Key == "legal_name" {
		if name, ok := f.Value.(string); ok {
			p.LegalName = name
		}
	}
	if f.Key == "status" {
		if st, ok := f.Value.(string); ok {
			p.Status = st
		}
	}
}

// GetFact retrieves a fact by its composite key.
func (p *Profile) GetFact(compositeKey string) (Fact, bool) {
	if p.Facts == nil {
		return Fact{}, false
	}
	f, exists := p.Facts[compositeKey]
	return f, exists
}
