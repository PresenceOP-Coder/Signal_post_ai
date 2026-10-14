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

type rawRolesResponse struct {
	Rollegrupper []struct {
		Type struct {
			Kode        string `json:"kode"`
			Beskrivelse string `json:"beskrivelse"`
		} `json:"type"`
		Roller []struct {
			Type struct {
				Kode        string `json:"kode"`
				Beskrivelse string `json:"beskrivelse"`
			} `json:"type"`
			Person *struct {
				Navn struct {
					Fornavn    string `json:"fornavn"`
					Mellomnavn string `json:"mellomnavn"`
					Etternavn  string `json:"etternavn"`
				} `json:"navn"`
			} `json:"person"`
			Enhet *struct {
				Organisasjonsnummer string `json:"organisasjonsnummer"`
				Navn                string `json:"navn"`
			} `json:"enhet"`
			Avregistrert bool `json:"avregistrert"`
		} `json:"roller"`
	} `json:"rollegrupper"`
}

// FetchRoles retrieves official governance roles (general manager, board chair, auditor) from Enhetsregisteret.
func FetchRoles(ctx context.Context, client *fetch.Client, orgnr string) ([]model.Fact, error) {
	url := fmt.Sprintf("https://data.brreg.no/enhetsregisteret/api/enheter/%s/roller", orgnr)
	resp, err := client.Get(ctx, url, 7*24*time.Hour)
	if err != nil {
		return nil, nil // non-fatal
	}

	if resp.StatusCode != 200 || len(resp.Body) == 0 {
		return nil, nil
	}

	var data rawRolesResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return nil, nil
	}

	var facts []model.Fact
	srcURL := resp.FinalURL
	if srcURL == "" {
		srcURL = url
	}
	fetchedAt := resp.FetchedAt

	for _, group := range data.Rollegrupper {
		for _, r := range group.Roller {
			if r.Avregistrert {
				continue
			}

			// Format person name
			var personName string
			if r.Person != nil {
				parts := []string{r.Person.Navn.Fornavn}
				if r.Person.Navn.Mellomnavn != "" {
					parts = append(parts, r.Person.Navn.Mellomnavn)
				}
				parts = append(parts, r.Person.Navn.Etternavn)
				personName = strings.TrimSpace(strings.Join(parts, " "))
			}

			// 1. General Manager (Daglig leder)
			if group.Type.Kode == "DAGL" || r.Type.Kode == "DAGL" {
				if personName != "" {
					f, err := model.NewFact("general_manager", personName, "", "", srcURL, fetchedAt, model.OriginRoles, fmt.Sprintf("Registered Daglig Leder (General Manager) in Enhetsregisteret for %s", orgnr))
					if err == nil {
						facts = append(facts, f)
					}
					break
				}
			}

			// 2. Chair of the Board (Styrets leder)
			if group.Type.Kode == "STYR" && (r.Type.Kode == "LEDE" || strings.Contains(strings.ToLower(r.Type.Beskrivelse), "leder")) {
				if personName != "" {
					f, err := model.NewFact("chair", personName, "", "", srcURL, fetchedAt, model.OriginRoles, fmt.Sprintf("Registered Styrets Leder (Board Chair) in Enhetsregisteret for %s", orgnr))
					if err == nil {
						facts = append(facts, f)
					}
					break
				}
			}

			// 3. Auditor (Revisor)
			if group.Type.Kode == "REVI" {
				var auditorName string
				if r.Enhet != nil && r.Enhet.Navn != "" {
					auditorName = strings.TrimSpace(r.Enhet.Navn)
				} else if personName != "" {
					auditorName = personName
				}
				if auditorName != "" {
					f, err := model.NewFact("auditor", auditorName, "", "", srcURL, fetchedAt, model.OriginRoles, fmt.Sprintf("Registered Revisor (Auditor) in Enhetsregisteret for %s", orgnr))
					if err == nil {
						facts = append(facts, f)
					}
					break
				}
			}
		}
	}

	return facts, nil
}
