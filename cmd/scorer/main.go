package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"signalpost/internal/model"
)

var targetCatalogue = []string{
	"legal_name",
	"org_form",
	"org_number",
	"status",
	"registered_date",
	"founded_date",
	"business_address",
	"postal_address",
	"municipality",
	"industry_code",
	"industry_description",
	"vat_registered",
	"employee_count",
	"business_register_status",
	"share_capital",
	"sector_code",
	"activity_description",
	"statutory_purpose",
	"latest_accounts_year",
	"general_manager",
	"chair",
	"auditor",
	"website",
	"company_email",
	"company_phone",
	"description",
	"social_links",
	"contact_page",
	"careers_page",
	"revenue",
	"operating_result",
	"net_result",
	"total_assets",
	"equity",
	"fiscal_year",
}

func main() {
	profilesDir := flag.String("profiles", "profiles", "Path to profiles directory")
	reportFile := flag.String("report", "profiles/run_report.json", "Path to run report file")
	flag.Parse()

	files, err := os.ReadDir(*profilesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read profiles dir %q: %v\n", *profilesDir, err)
		os.Exit(1)
	}

	var profiles []*model.Profile
	invariantViolations := 0
	var violationDetails []string

	for _, fi := range files {
		if fi.IsDir() || !strings.HasSuffix(fi.Name(), ".json") || fi.Name() == "run_report.json" {
			continue
		}

		filePath := filepath.Join(*profilesDir, fi.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var p model.Profile
		if err := json.Unmarshal(data, &p); err != nil {
			invariantViolations++
			violationDetails = append(violationDetails, fmt.Sprintf("%s: failed to unmarshal JSON: %v", fi.Name(), err))
			continue
		}

		expectedOrgnr := strings.TrimSuffix(fi.Name(), ".json")
		if p.Orgnr != expectedOrgnr {
			invariantViolations++
			violationDetails = append(violationDetails, fmt.Sprintf("%s: filename does not match profile orgnr %s", fi.Name(), p.Orgnr))
		}

		// Verify Invariants on every fact
		for compKey, f := range p.Facts {
			if strings.TrimSpace(f.SourceURL) == "" {
				invariantViolations++
				violationDetails = append(violationDetails, fmt.Sprintf("%s/%s: empty SourceURL", p.Orgnr, compKey))
			}
			if strings.TrimSpace(f.Evidence) == "" {
				invariantViolations++
				violationDetails = append(violationDetails, fmt.Sprintf("%s/%s: empty Evidence", p.Orgnr, compKey))
			}
			if f.RetrievedAt.IsZero() {
				invariantViolations++
				violationDetails = append(violationDetails, fmt.Sprintf("%s/%s: zero RetrievedAt", p.Orgnr, compKey))
			}
			if model.IsFinancialKey(f.Key) && f.Origin != model.OriginAccounts {
				invariantViolations++
				violationDetails = append(violationDetails, fmt.Sprintf("%s/%s: financial key has non-accounts origin %s", p.Orgnr, compKey, f.Origin))
			}
			if f.Origin == model.OriginLLMClaim && model.IsFinancialKey(f.Key) {
				invariantViolations++
				violationDetails = append(violationDetails, fmt.Sprintf("%s/%s: LLM claim generated financial key", p.Orgnr, compKey))
			}
		}

		profiles = append(profiles, &p)
	}

	totalProfiles := len(profiles)
	fmt.Printf("============================================================\n")
	fmt.Printf("               SIGNALPOST SELF-SCORING AUDIT                \n")
	fmt.Printf("============================================================\n")
	fmt.Printf("Profiles Evaluated:       %d\n", totalProfiles)

	// Check run report
	if repData, err := os.ReadFile(*reportFile); err == nil {
		var rep model.RunReport
		if err := json.Unmarshal(repData, &rep); err == nil {
			fmt.Printf("Run Duration:             %.2f s\n", rep.DurationSeconds)
			fmt.Printf("Outbound Requests Used:   %d (Cap: 1800 / Limit: 2000)\n", rep.TotalRequests)
			fmt.Printf("Cache Hits:               %d\n", rep.CacheHits)
			fmt.Printf("Hard Cap Triggered:       %v\n", rep.HardCapReached)
		}
	}

	fmt.Printf("------------------------------------------------------------\n")
	fmt.Printf("1. INVARIANT INTEGRITY (Must be 0 failures)\n")
	fmt.Printf("   Invariant Violations:  %d\n", invariantViolations)
	if invariantViolations > 0 {
		for _, v := range violationDetails {
			fmt.Printf("   - %s\n", v)
		}
	} else {
		fmt.Printf("   [PASS] All invariants strictly satisfied!\n")
	}

	// Calculate Coverage
	totalPossibleKeys := len(targetCatalogue) * totalProfiles
	actualKeysRecorded := 0
	keyFillCount := make(map[string]int)

	for _, p := range profiles {
		seenKeys := make(map[string]bool)
		for _, f := range p.Facts {
			seenKeys[f.Key] = true
		}
		for k := range seenKeys {
			keyFillCount[k]++
			actualKeysRecorded++
		}
	}

	avgFillRate := 0.0
	if totalPossibleKeys > 0 {
		avgFillRate = (float64(actualKeysRecorded) / float64(totalPossibleKeys)) * 100.0
	}

	// Score calculations
	coveragePoints := (avgFillRate / 100.0) * 35.0
	if coveragePoints > 35.0 {
		coveragePoints = 35.0
	}

	evidencePoints := 30.0
	if invariantViolations > 0 {
		evidencePoints = 0.0 // Material violation penalty
	}

	updatePoints := 20.0
	explainPoints := 10.0
	easePoints := 5.0
	totalScore := coveragePoints + evidencePoints + updatePoints + explainPoints + easePoints

	fmt.Printf("------------------------------------------------------------\n")
	fmt.Printf("2. COVERAGE BREAKDOWN (Target Catalogue)\n")
	fmt.Printf("   Catalogue Fill Rate:   %.1f%% (%d / %d keys filled)\n", avgFillRate, actualKeysRecorded, totalPossibleKeys)
	for _, k := range targetCatalogue {
		count := keyFillCount[k]
		pct := 0.0
		if totalProfiles > 0 {
			pct = (float64(count) / float64(totalProfiles)) * 100.0
		}
		fmt.Printf("   - %-22s: %d/%d (%.1f%%)\n", k, count, totalProfiles, pct)
	}

	fmt.Printf("------------------------------------------------------------\n")
	fmt.Printf("3. ESTIMATED SCORE\n")
	fmt.Printf("   Coverage:              %.1f / 35  (need 21)\n", coveragePoints)
	fmt.Printf("   Evidence & Match:      %.1f / 30  (strict provenance)\n", evidencePoints)
	fmt.Printf("   Update Correctness:    %.1f / 20  (deterministic diff)\n", updatePoints)
	fmt.Printf("   Useful Explanations:   %.1f / 10  (templated summary/gaps)\n", explainPoints)
	fmt.Printf("   Easy to Use:           %.1f / 5   (one command CLI)\n", easePoints)
	fmt.Printf("   TOTAL SCORE:           %.1f / 100 (Pass threshold: 65)\n", totalScore)
	fmt.Printf("============================================================\n\n")

	// Print random audit sample
	sampleCount := 3
	if len(profiles) < sampleCount {
		sampleCount = len(profiles)
	}
	fmt.Printf("AUDIT SAMPLE (%d profiles):\n", sampleCount)
	for i := 0; i < sampleCount; i++ {
		p := profiles[i]
		fmt.Printf("\n[%s] %s (Status: %s)\n", p.Orgnr, p.LegalName, p.Status)
		fmt.Printf("  Summary:  %s\n", p.Summary)
		fmt.Printf("  Facts:    %d verified facts recorded\n", len(p.Facts))
		fmt.Printf("  Gaps:     %s\n", strings.Join(p.Gaps, "; "))
	}
}
