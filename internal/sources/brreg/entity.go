package brreg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"signalpost/internal/fetch"
	"signalpost/internal/model"
)

var (
	ErrEntityNotFound = errors.New("entity not found in Enhetsregisteret")
	ErrOrgnrMismatch  = errors.New("registry response orgnr does not match requested orgnr")
)

type rawAddress struct {
	Adresse       []string `json:"adresse"`
	Postnummer    string   `json:"postnummer"`
	Poststed      string   `json:"poststed"`
	Kommune       string   `json:"kommune"`
	Kommunenummer string   `json:"kommunenummer"`
	Land          string   `json:"land"`
	Landkode      string   `json:"landkode"`
}

func (a *rawAddress) Format() string {
	if a == nil {
		return ""
	}
	parts := make([]string, 0, len(a.Adresse)+3)
	for _, l := range a.Adresse {
		l = strings.TrimSpace(l)
		if l != "" {
			parts = append(parts, l)
		}
	}
	cityLine := strings.TrimSpace(fmt.Sprintf("%s %s", a.Postnummer, a.Poststed))
	if cityLine != "" {
		parts = append(parts, cityLine)
	}
	if a.Land != "" && a.Land != "Norge" {
		parts = append(parts, a.Land)
	}
	return strings.Join(parts, ", ")
}

type rawEnhetResponse struct {
	Organisasjonsnummer                      string      `json:"organisasjonsnummer"`
	Navn                                     string      `json:"navn"`
	Organisasjonsform                        *struct {
		Kode        string `json:"kode"`
		Beskrivelse string `json:"beskrivelse"`
	} `json:"organisasjonsform"`
	RegistreringsdatoEnhetsregisteret        string      `json:"registreringsdatoEnhetsregisteret"`
	Stiftelsesdato                           string      `json:"stiftelsesdato"`
	Hjemmeside                               string      `json:"hjemmeside"`
	Epost                                    string      `json:"epost"`
	Telefon                                  string      `json:"telefon"`
	AntallAnsatte                            *int        `json:"antallAnsatte"`
	HarRegistrertAntallAnsatte               bool        `json:"harRegistrertAntallAnsatte"`
	RegistrertIMvaregisteret                 *bool       `json:"registrertIMvaregisteret"`
	Forretningsadresse                       *rawAddress `json:"forretningsadresse"`
	Postadresse                              *rawAddress `json:"postadresse"`
	Naeringskode1                            *struct {
		Kode        string `json:"kode"`
		Beskrivelse string `json:"beskrivelse"`
	} `json:"naeringskode1"`
	Konkurs                                  bool        `json:"konkurs"`
	UnderAvvikling                           bool        `json:"underAvvikling"`
	UnderTvangsavviklingEllerTvangsopplosning bool        `json:"underTvangsavviklingEllerTvangsopplosning"`
	Slettedato                               string      `json:"slettedato"`
	Aktivitet                                []string    `json:"aktivitet"`
	VedtektsfestetFormaal                    []string    `json:"vedtektsfestetFormaal"`
	RegistrertIForetaksregisteret            *bool       `json:"registrertIForetaksregisteret"`
	Kapital                                  *struct {
		Belop  *float64 `json:"belop"`
		Valuta string   `json:"valuta"`
		Type   string   `json:"type"`
	} `json:"kapital"`
	InstitusjonellSektorkode                 *struct {
		Kode        string `json:"kode"`
		Beskrivelse string `json:"beskrivelse"`
	} `json:"institusjonellSektorkode"`
	SisteInnsendteAarsregnskap               string      `json:"sisteInnsendteAarsregnskap"`
}

// FetchEntity fetches official registration data from Enhetsregisteret.
func FetchEntity(ctx context.Context, client *fetch.Client, orgnr string) ([]model.Fact, error) {
	primaryURL := fmt.Sprintf("https://data.brreg.no/enhetsregisteret/api/enheter/%s", orgnr)
	resp, err := client.Get(ctx, primaryURL, 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch enhet %s: %w", orgnr, err)
	}

	// Handle 410 Gone (deleted entity)
	if resp.StatusCode == 410 {
		f, fErr := model.NewFact("status", "deleted", "", "", primaryURL, resp.FetchedAt, model.OriginRegistry, fmt.Sprintf("Enhetsregisteret HTTP 410 confirms %s was deleted", orgnr))
		if fErr != nil {
			return nil, fErr
		}
		return []model.Fact{f}, nil
	}

	// Handle 404 - Check sub-units (underenheter)
	if resp.StatusCode == 404 {
		subURL := fmt.Sprintf("https://data.brreg.no/enhetsregisteret/api/underenheter/%s", orgnr)
		resp, err = client.Get(ctx, subURL, 24*time.Hour)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch underenhet %s: %w", orgnr, err)
		}
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("%w: %s", ErrEntityNotFound, orgnr)
		}
		if resp.StatusCode == 410 {
			f, fErr := model.NewFact("status", "deleted", "", "", subURL, resp.FetchedAt, model.OriginRegistry, fmt.Sprintf("Enhetsregisteret underenhet HTTP 410 confirms %s was deleted", orgnr))
			if fErr != nil {
				return nil, fErr
			}
			return []model.Fact{f}, nil
		}
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status %d from Enhetsregisteret for %s", resp.StatusCode, orgnr)
	}

	var data rawEnhetResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse enhet JSON for %s: %w", orgnr, err)
	}

	// Invariant: Response orgnr must match requested orgnr
	if data.Organisasjonsnummer != orgnr {
		return nil, fmt.Errorf("%w: requested %s but got %s", ErrOrgnrMismatch, orgnr, data.Organisasjonsnummer)
	}

	var facts []model.Fact
	srcURL := resp.FinalURL
	if srcURL == "" {
		srcURL = primaryURL
	}
	fetchedAt := resp.FetchedAt

	add := func(key string, val any, unit, evidence string) {
		if val == nil {
			return
		}
		if s, ok := val.(string); ok && strings.TrimSpace(s) == "" {
			return
		}
		f, err := model.NewFact(key, val, unit, "", srcURL, fetchedAt, model.OriginRegistry, evidence)
		if err == nil {
			facts = append(facts, f)
		}
	}

	// Identity facts
	add("org_number", data.Organisasjonsnummer, "", fmt.Sprintf("Official registration number in Enhetsregisteret: %s", orgnr))
	add("legal_name", data.Navn, "", fmt.Sprintf("Official registered legal name in Enhetsregisteret for %s", orgnr))

	if data.Organisasjonsform != nil {
		add("org_form", data.Organisasjonsform.Kode, "", fmt.Sprintf("Registered corporate legal form: %s (%s)", data.Organisasjonsform.Kode, data.Organisasjonsform.Beskrivelse))
	}

	// Entity status determination
	status := "active"
	statusReason := "Active entity in Enhetsregisteret"
	if data.Slettedato != "" {
		status = "deleted"
		statusReason = fmt.Sprintf("Entity deleted on %s according to Enhetsregisteret", data.Slettedato)
	} else if data.Konkurs {
		status = "bankrupt"
		statusReason = "Registered as bankrupt in Enhetsregisteret"
	} else if data.UnderAvvikling || data.UnderTvangsavviklingEllerTvangsopplosning {
		status = "under_liquidation"
		statusReason = "Under liquidation according to Enhetsregisteret"
	}
	add("status", status, "", statusReason)

	if data.RegistreringsdatoEnhetsregisteret != "" {
		add("registered_date", data.RegistreringsdatoEnhetsregisteret, "", "Date registered in Enhetsregisteret")
	}
	if data.Stiftelsesdato != "" {
		add("founded_date", data.Stiftelsesdato, "", "Official founding date registered with Enhetsregisteret")
	}

	// Address facts
	if data.Forretningsadresse != nil {
		formatted := data.Forretningsadresse.Format()
		if formatted != "" {
			add("business_address", formatted, "", "Official business address registered with Enhetsregisteret")
		}
		if data.Forretningsadresse.Kommune != "" {
			add("municipality", data.Forretningsadresse.Kommune, "", "Municipality of business address")
		}
	}
	if data.Postadresse != nil {
		formatted := data.Postadresse.Format()
		if formatted != "" {
			add("postal_address", formatted, "", "Official postal address registered with Enhetsregisteret")
		}
		if data.Forretningsadresse == nil && data.Postadresse.Kommune != "" {
			add("municipality", data.Postadresse.Kommune, "", "Municipality of postal address")
		}
	}

	// Activity facts
	if data.Naeringskode1 != nil && data.Naeringskode1.Kode != "" {
		add("industry_code", data.Naeringskode1.Kode, "", "Primary NACE industry classification code")
		if data.Naeringskode1.Beskrivelse != "" {
			add("industry_description", data.Naeringskode1.Beskrivelse, "", "Primary NACE industry classification description")
		}
	}
	if data.RegistrertIMvaregisteret != nil {
		add("vat_registered", *data.RegistrertIMvaregisteret, "", "Value Added Tax (MVA) register status")
	}
	if data.AntallAnsatte != nil && data.HarRegistrertAntallAnsatte {
		add("employee_count", *data.AntallAnsatte, "count", "Registered employee count in NAV/Enhetsregisteret")
	}

	// Contact facts
	if data.Hjemmeside != "" {
		// Clean URL if prefix missing
		ws := strings.TrimSpace(data.Hjemmeside)
		if !strings.HasPrefix(ws, "http://") && !strings.HasPrefix(ws, "https://") {
			ws = "https://" + ws
		}
		add("website", ws, "", "Official website listed in Enhetsregisteret")
	}
	if data.Telefon != "" {
		add("company_phone", strings.TrimSpace(data.Telefon), "", "Official telephone listed in Enhetsregisteret")
	}
	if data.Epost != "" {
		add("company_email", strings.TrimSpace(data.Epost), "", "Official email listed in Enhetsregisteret")
	}

	// Business activity & statutory purpose
	if len(data.Aktivitet) > 0 {
		act := strings.TrimSpace(strings.Join(data.Aktivitet, " "))
		if act != "" {
			add("activity_description", act, "", "Registered business activity description in Enhetsregisteret")
		}
	}
	if len(data.VedtektsfestetFormaal) > 0 {
		form := strings.TrimSpace(strings.Join(data.VedtektsfestetFormaal, " "))
		if form != "" {
			add("statutory_purpose", form, "", "Statutory corporate purpose registered with Enhetsregisteret")
		}
	}

	// Commercial register status
	if data.RegistrertIForetaksregisteret != nil {
		add("business_register_status", *data.RegistrertIForetaksregisteret, "", "Foretaksregisteret (Register of Business Enterprises) status")
	}

	// Share capital
	if data.Kapital != nil && data.Kapital.Belop != nil {
		curr := strings.TrimSpace(data.Kapital.Valuta)
		if curr == "" {
			curr = "NOK"
		}
		add("share_capital", *data.Kapital.Belop, curr, fmt.Sprintf("Registered share capital (%s)", data.Kapital.Type))
	}

	// Institutional sector
	if data.InstitusjonellSektorkode != nil && data.InstitusjonellSektorkode.Kode != "" {
		add("sector_code", data.InstitusjonellSektorkode.Kode, "", "Institutional sector classification code")
		if data.InstitusjonellSektorkode.Beskrivelse != "" {
			add("sector_description", data.InstitusjonellSektorkode.Beskrivelse, "", "Institutional sector description")
		}
	}

	// Latest annual accounts filing year
	if data.SisteInnsendteAarsregnskap != "" {
		add("latest_accounts_year", data.SisteInnsendteAarsregnskap, "", "Year of latest annual accounts submitted to registry")
	}

	return facts, nil
}
