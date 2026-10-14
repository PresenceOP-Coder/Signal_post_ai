package explain

import (
	"fmt"
	"strings"

	"signalpost/internal/model"
)

// GenerateExplanations populates the profile's Summary, ChangesSummary, and Gaps
// using deterministic, high-integrity templates without LLM hallucination.
func GenerateExplanations(p *model.Profile) {
	if p == nil {
		return
	}

	// 1. Build Summary
	status := "Active"
	if f, ok := p.GetFact("status"); ok {
		if s, ok := f.Value.(string); ok && s != "" {
			status = strings.Title(strings.ReplaceAll(s, "_", " "))
		}
	}

	orgForm := ""
	if f, ok := p.GetFact("org_form"); ok {
		if s, ok := f.Value.(string); ok {
			orgForm = s
		}
	}

	muni := ""
	if f, ok := p.GetFact("municipality"); ok {
		if s, ok := f.Value.(string); ok {
			muni = s
		}
	}

	nace := ""
	if f, ok := p.GetFact("industry_code"); ok {
		if s, ok := f.Value.(string); ok {
			nace = s
		}
	}

	empStr := "Employee count not registered"
	if f, ok := p.GetFact("employee_count"); ok {
		empStr = fmt.Sprintf("%v employees", f.Value)
	}

	accStr := "No annual accounts on file"
	// Find latest fiscal year
	var latestYear string
	for _, f := range p.Facts {
		if f.Key == "fiscal_year" && f.Period > latestYear {
			latestYear = f.Period
		}
	}
	if latestYear != "" {
		accStr = fmt.Sprintf("Annual accounts for FY%s on file", latestYear)
	}

	var summaryParts []string
	if orgForm != "" && muni != "" {
		summaryParts = append(summaryParts, fmt.Sprintf("%s %s in %s", status, orgForm, muni))
	} else if muni != "" {
		summaryParts = append(summaryParts, fmt.Sprintf("%s entity in %s", status, muni))
	} else {
		summaryParts = append(summaryParts, fmt.Sprintf("%s entity", status))
	}

	if nace != "" {
		summaryParts = append(summaryParts, fmt.Sprintf("NACE %s", nace))
	}
	summaryParts = append(summaryParts, empStr)

	summary := strings.Join(summaryParts, ", ") + ". " + accStr + "."
	p.Summary = summary

	// 2. Build ChangesSummary
	p.ChangesSummary = make([]string, 0)
	if len(p.History) == 0 {
		p.ChangesSummary = append(p.ChangesSummary, "Baseline profile established; verified against official registries.")
	} else {
		for _, chg := range p.History {
			timeStr := chg.ChangedAt.Format("2006-01-02")
			if chg.OldValue == nil {
				p.ChangesSummary = append(p.ChangesSummary, fmt.Sprintf("New fact %s recorded as %v on %s (%s)", chg.Key, chg.NewValue, timeStr, chg.SourceURL))
			} else {
				p.ChangesSummary = append(p.ChangesSummary, fmt.Sprintf("Field %s updated from %v to %v on %s (%s)", chg.Key, chg.OldValue, chg.NewValue, timeStr, chg.SourceURL))
			}
		}
	}

	// 3. Build Gaps
	p.Gaps = make([]string, 0)
	if _, ok := p.GetFact("website"); !ok {
		p.Gaps = append(p.Gaps, "Official website not listed in Enhetsregisteret.")
	}
	if latestYear == "" {
		p.Gaps = append(p.Gaps, "No annual accounts filed with Regnskapsregisteret (common for non-commercial or exempt entities).")
	}
	if _, ok := p.GetFact("employee_count"); !ok {
		p.Gaps = append(p.Gaps, "Employee count not reported in Aa-register.")
	}
	if _, ok := p.GetFact("company_email"); !ok {
		p.Gaps = append(p.Gaps, "Email address not published in registry record.")
	}
	if _, ok := p.GetFact("company_phone"); !ok {
		p.Gaps = append(p.Gaps, "Telephone number not published in registry record.")
	}
}
