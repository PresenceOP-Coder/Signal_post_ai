package verify

import (
	"errors"
	"testing"
)

func TestPageContainsOrgnr(t *testing.T) {
	orgnr := "923609016"
	tests := []struct {
		text     string
		expected bool
	}{
		{"Contact us: Org nr 923609016 MVA", true},
		{"Org.nr: 923 609 016", true},
		{"Foretaksregisteret: 923.609.016", true},
		{"Random text with 123456789", false},
		{"", false},
	}

	for _, tt := range tests {
		got := PageContainsOrgnr(tt.text, orgnr)
		if got != tt.expected {
			t.Errorf("PageContainsOrgnr(%q) = %v; want %v", tt.text, got, tt.expected)
		}
	}
}

func TestVerifyPageAcceptanceRules(t *testing.T) {
	orgnr := "923609016"
	companyName := "Equinor ASA"
	regWebsite := "https://www.equinor.com"

	// 1. Accepted by orgnr anywhere on page
	ok, reason := VerifyPage("Copyright 2026. Org: 923 609 016", "https://otherdomain.no", regWebsite, companyName, "4035", "Stavanger", orgnr)
	if !ok {
		t.Errorf("Expected acceptance when orgnr present on page, got false (%s)", reason)
	}

	// 2. Accepted by domain + name match
	ok, reason = VerifyPage("Welcome to Equinor official portal", "https://www.equinor.com/about", regWebsite, companyName, "4035", "Stavanger", orgnr)
	if !ok {
		t.Errorf("Expected acceptance when domain + name match, got false (%s)", reason)
	}

	// 3. Rejected: Name-only match on completely different domain
	ok, _ = VerifyPage("We do business with Equinor regularly", "https://some-blog.no/post", regWebsite, companyName, "", "", orgnr)
	if ok {
		t.Errorf("Name-only match on unrelated domain must be strictly rejected")
	}

	// 4. Rejected: Same domain but zero mention of company name or orgnr
	ok, _ = VerifyPage("Lorem ipsum dolor sit amet", "https://www.equinor.com/empty", regWebsite, companyName, "", "", orgnr)
	if ok {
		t.Errorf("Matching domain with zero company identification must be rejected")
	}
}

func TestVerifyLLMClaim(t *testing.T) {
	pageText := "Equinor is an international energy company committed to long-term value creation in a low-carbon future. Headquartered in Stavanger."

	// 1. Valid exact snippet
	snippet := "committed to long-term value creation in a low-carbon future"
	if err := VerifyLLMClaim(snippet, pageText); err != nil {
		t.Errorf("Valid snippet failed verification: %v", err)
	}

	// 2. Snippet not in page
	badSnippet := "Equinor was founded by Elon Musk in 2010"
	if err := VerifyLLMClaim(badSnippet, pageText); !errors.Is(err, ErrSnippetNotFound) {
		t.Errorf("Expected ErrSnippetNotFound, got: %v", err)
	}

	// 3. Too short snippet
	short := "Equinor is"
	if err := VerifyLLMClaim(short, pageText); err == nil {
		t.Errorf("Too short snippet should be rejected")
	}
}
