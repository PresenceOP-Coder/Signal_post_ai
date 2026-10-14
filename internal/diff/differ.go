package diff

import (
	"fmt"
	"reflect"
	"time"

	"signalpost/internal/model"
)

// DiffResult holds detected changes between prior profile and current facts.
type DiffResult struct {
	Changes []model.Change
}

// DiffAndApply updates a profile by comparing existing facts with newly extracted facts,
// preserving history and updating LastVerified and LastChanged according to Invariant 7.
func DiffAndApply(prior *model.Profile, orgnr string, newFacts []model.Fact, verificationTime time.Time) *model.Profile {
	var current *model.Profile

	if prior == nil {
		current = model.NewProfile(orgnr)
		current.CreatedAt = verificationTime
		current.LastVerified = verificationTime
		current.LastChanged = verificationTime

		for _, f := range newFacts {
			current.AddFact(f)
		}
		return current
	}

	// Clone prior profile
	current = &model.Profile{
		Orgnr:          prior.Orgnr,
		LegalName:      prior.LegalName,
		Status:         prior.Status,
		Facts:          make(map[string]model.Fact),
		History:        make([]model.Change, len(prior.History)),
		Summary:        prior.Summary,
		ChangesSummary: make([]string, 0),
		Gaps:           make([]string, 0),
		LastVerified:   verificationTime,
		LastChanged:    prior.LastChanged,
		CreatedAt:      prior.CreatedAt,
	}
	copy(current.History, prior.History)

	// Copy prior facts initially
	for k, v := range prior.Facts {
		current.Facts[k] = v
	}

	var newChanges []model.Change

	for _, f := range newFacts {
		compositeKey := f.CompositeKey()
		priorFact, exists := prior.Facts[compositeKey]

		if !exists {
			// Newly discovered fact
			chg := model.Change{
				Key:       f.Key,
				Period:    f.Period,
				OldValue:  nil,
				NewValue:  f.Value,
				ChangedAt: verificationTime,
				SourceURL: f.SourceURL,
				Reason:    fmt.Sprintf("Fact %s added", compositeKey),
			}
			newChanges = append(newChanges, chg)
			current.AddFact(f)
		} else if !valuesEqual(priorFact.Value, f.Value) {
			// Value changed
			chg := model.Change{
				Key:       f.Key,
				Period:    f.Period,
				OldValue:  priorFact.Value,
				NewValue:  f.Value,
				ChangedAt: verificationTime,
				SourceURL: f.SourceURL,
				Reason:    fmt.Sprintf("Value changed from %v to %v", priorFact.Value, f.Value),
			}
			newChanges = append(newChanges, chg)
			current.AddFact(f)
		} else {
			// Value identical: update fact to reflect latest verification timestamp and source
			current.AddFact(f)
		}
	}

	if len(newChanges) > 0 {
		current.LastChanged = verificationTime
		current.History = append(current.History, newChanges...)
	}

	return current
}

func valuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// For numbers, compare float values to avoid int vs float64 json unmarshal inequality
	if aNum, okA := toFloat(a); okA {
		if bNum, okB := toFloat(b); okB {
			return aNum == bNum
		}
	}
	return reflect.DeepEqual(a, b)
}

func toFloat(val any) (float64, bool) {
	switch v := val.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	default:
		return 0, false
	}
}
