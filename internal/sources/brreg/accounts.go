package brreg

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"signalpost/internal/fetch"
	"signalpost/internal/model"
)

type rawAccountItem struct {
	ID         int64  `json:"id"`
	Journalnr  string `json:"journalnr"`
	Virksomhet struct {
		Organisasjonsnummer string `json:"organisasjonsnummer"`
		Organisasjonsform   string `json:"organisasjonsform"`
	} `json:"virksomhet"`
	Regnskapstype   string `json:"regnskapstype"` // "SELSKAP" or "KONSERN"
	Regnskapsperiode struct {
		FraDato string `json:"fraDato"`
		TilDato string `json:"tilDato"`
	} `json:"regnskapsperiode"`
	Valuta    string `json:"valuta"`
	Eiendeler *struct {
		SumEiendeler *float64 `json:"sumEiendeler"`
	} `json:"eiendeler"`
	EgenkapitalGjeld *struct {
		Egenkapital *struct {
			SumEgenkapital *float64 `json:"sumEgenkapital"`
		} `json:"egenkapital"`
	} `json:"egenkapitalGjeld"`
	ResultatregnskapResultat *struct {
		Aarsresultat   *float64 `json:"aarsresultat"`
		Driftsresultat *struct {
			Driftsresultat  *float64 `json:"driftsresultat"`
			Driftsinntekter *struct {
				SumDriftsinntekter *float64 `json:"sumDriftsinntekter"`
				Salgsinntekter     *float64 `json:"salgsinntekter"`
			} `json:"driftsinntekter"`
		} `json:"driftsresultat"`
	} `json:"resultatregnskapResultat"`
}

// FetchAccounts retrieves official annual accounting statements from Regnskapsregisteret.
func FetchAccounts(ctx context.Context, client *fetch.Client, orgnr string) ([]model.Fact, error) {
	url := fmt.Sprintf("https://data.brreg.no/regnskapsregisteret/regnskap/%s", orgnr)
	resp, err := client.Get(ctx, url, 7*24*time.Hour) // 7-day cache TTL for accounts
	if err != nil {
		// Non-fatal: some entities cause 500 or are unavailable
		return nil, nil
	}

	if resp.StatusCode == 404 || len(resp.Body) == 0 {
		// Small, exempt, or non-commercial entities have no accounts on file
		return nil, nil
	}

	if resp.StatusCode != 200 {
		return nil, nil
	}

	var items []rawAccountItem
	if err := json.Unmarshal(resp.Body, &items); err != nil {
		return nil, fmt.Errorf("failed to parse regnskap JSON for %s: %w", orgnr, err)
	}

	var facts []model.Fact
	srcURL := resp.FinalURL
	if srcURL == "" {
		srcURL = url
	}
	fetchedAt := resp.FetchedAt

	// Sort / organize items: keep company-level (SELSKAP) over KONSERN if both exist for same year
	seenYear := make(map[string]bool)

	// First pass: SELSKAP
	for _, it := range items {
		if it.Regnskapstype == "SELSKAP" {
			extractFactsFromItem(it, orgnr, srcURL, fetchedAt, &facts, seenYear)
		}
	}

	// Second pass: other types (e.g. KONSERN) if year not already extracted
	for _, it := range items {
		if it.Regnskapstype != "SELSKAP" {
			extractFactsFromItem(it, orgnr, srcURL, fetchedAt, &facts, seenYear)
		}
	}

	return facts, nil
}

func extractFactsFromItem(it rawAccountItem, orgnr, srcURL string, fetchedAt time.Time, facts *[]model.Fact, seenYear map[string]bool) {
	// Invariant: Verify orgnr matches
	if it.Virksomhet.Organisasjonsnummer != "" && it.Virksomhet.Organisasjonsnummer != orgnr {
		return
	}

	tilDato := strings.TrimSpace(it.Regnskapsperiode.TilDato)
	if len(tilDato) < 4 {
		return
	}
	year := tilDato[:4]
	if seenYear[year] {
		return
	}
	seenYear[year] = true

	currency := strings.TrimSpace(it.Valuta)
	if currency == "" {
		currency = "NOK"
	}

	evidence := fmt.Sprintf("Regnskapsregisteret official %s annual accounts for %s (FY%s)", it.Regnskapstype, orgnr, year)

	addFin := func(key string, val float64) {
		f, err := model.NewFact(key, val, currency, year, srcURL, fetchedAt, model.OriginAccounts, evidence)
		if err == nil {
			*facts = append(*facts, f)
		}
	}

	// Fiscal year fact
	if fYear, err := model.NewFact("fiscal_year", year, "", year, srcURL, fetchedAt, model.OriginAccounts, evidence); err == nil {
		*facts = append(*facts, fYear)
	}

	// Balance sheet: Total Assets & Equity
	if it.Eiendeler != nil && it.Eiendeler.SumEiendeler != nil {
		addFin("total_assets", *it.Eiendeler.SumEiendeler)
	}
	if it.EgenkapitalGjeld != nil && it.EgenkapitalGjeld.Egenkapital != nil && it.EgenkapitalGjeld.Egenkapital.SumEgenkapital != nil {
		addFin("equity", *it.EgenkapitalGjeld.Egenkapital.SumEgenkapital)
	}

	// Income statement: Revenue, Operating result, Net result
	if it.ResultatregnskapResultat != nil {
		if it.ResultatregnskapResultat.Aarsresultat != nil {
			addFin("net_result", *it.ResultatregnskapResultat.Aarsresultat)
		}
		if it.ResultatregnskapResultat.Driftsresultat != nil {
			dr := it.ResultatregnskapResultat.Driftsresultat
			if dr.Driftsresultat != nil {
				addFin("operating_result", *dr.Driftsresultat)
			}
			if dr.Driftsinntekter != nil {
				if dr.Driftsinntekter.SumDriftsinntekter != nil {
					addFin("revenue", *dr.Driftsinntekter.SumDriftsinntekter)
				} else if dr.Driftsinntekter.Salgsinntekter != nil {
					addFin("revenue", *dr.Driftsinntekter.Salgsinntekter)
				}
			}
		}
	}
}
