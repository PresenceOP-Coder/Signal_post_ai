package diff

import (
	"testing"
	"time"

	"signalpost/internal/model"
)

func TestDiffAndApplyInvariant7(t *testing.T) {
	t1 := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)

	orgnr := "923609016"
	f1, _ := model.NewFact("legal_name", "EQUINOR ASA", "", "", "https://brreg.no", t1, model.OriginRegistry, "evidence")
	f2, _ := model.NewFact("employee_count", 20000, "count", "", "https://brreg.no", t1, model.OriginRegistry, "evidence")

	// 1. First run creates initial profile
	p1 := DiffAndApply(nil, orgnr, []model.Fact{f1, f2}, t1)
	if p1.LegalName != "EQUINOR ASA" {
		t.Errorf("LegalName = %q; want EQUINOR ASA", p1.LegalName)
	}
	if len(p1.History) != 0 {
		t.Errorf("Initial run should not have change history, got %d items", len(p1.History))
	}
	if !p1.LastVerified.Equal(t1) || !p1.LastChanged.Equal(t1) {
		t.Errorf("Initial timestamps mismatch")
	}

	// 2. Second run with IDENTICAL facts (Invariant 7: empty change list, updates only last_verified)
	f1Same, _ := model.NewFact("legal_name", "EQUINOR ASA", "", "", "https://brreg.no", t2, model.OriginRegistry, "evidence")
	f2Same, _ := model.NewFact("employee_count", 20000, "count", "", "https://brreg.no", t2, model.OriginRegistry, "evidence")

	p2 := DiffAndApply(p1, orgnr, []model.Fact{f1Same, f2Same}, t2)
	if len(p2.History) != 0 {
		t.Errorf("Invariant 7 violated: re-running unchanged inputs added %d history items", len(p2.History))
	}
	if !p2.LastVerified.Equal(t2) {
		t.Errorf("LastVerified should be updated to t2")
	}
	if !p2.LastChanged.Equal(t1) {
		t.Errorf("LastChanged should remain t1 when unchanged, got %v", p2.LastChanged)
	}

	// 3. Third run with MODIFIED fact (employee count increased to 21000)
	f2Changed, _ := model.NewFact("employee_count", 21000, "count", "", "https://brreg.no", t3, model.OriginRegistry, "evidence")
	p3 := DiffAndApply(p2, orgnr, []model.Fact{f1Same, f2Changed}, t3)

	if len(p3.History) != 1 {
		t.Fatalf("Expected 1 history item after modification, got %d", len(p3.History))
	}
	chg := p3.History[0]
	if chg.Key != "employee_count" {
		t.Errorf("Change key = %q; want employee_count", chg.Key)
	}
	if chg.OldValue != 20000 || chg.NewValue != 21000 {
		t.Errorf("Change values mismatch: %v -> %v", chg.OldValue, chg.NewValue)
	}
	if !p3.LastChanged.Equal(t3) {
		t.Errorf("LastChanged should be updated to t3 upon modification")
	}
}
