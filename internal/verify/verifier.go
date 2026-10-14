package verify

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"signalpost/internal/model"
)

var (
	ErrUnverifiedOrigin   = errors.New("unverified fact origin")
	ErrPageNotAccepted    = errors.New("page does not satisfy company verification rules")
	ErrSnippetNotFound    = errors.New("claim verbatim snippet not found in page text")
	ErrNumericLLMClaim    = errors.New("numeric claims from LLM are strictly disallowed")
)

// VerifyFact checks that a Fact complies with origin and identity rules.
func VerifyFact(f model.Fact, targetOrgnr string) error {
	if f.Origin == model.OriginRegistry || f.Origin == model.OriginAccounts {
		if !strings.Contains(f.SourceURL, targetOrgnr) && !strings.Contains(f.Evidence, targetOrgnr) {
			return errors.New("registry fact does not match target company orgnr")
		}
	}
	return nil
}

var (
	corpSuffixRegex = regexp.MustCompile(`\b(asa|as|enk|da|ans|ba|sa|ks|nuf)\b`)
	nonAlphaRegex   = regexp.MustCompile(`[^a-z0-9æøå]+`)
)

// NormalizeName normalizes Norwegian company names for fuzzy substring matching:
// trims spaces, lowercases, removes punctuation, and strips common legal suffixes (AS, ASA, etc.)
func NormalizeName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = corpSuffixRegex.ReplaceAllString(s, "")
	s = nonAlphaRegex.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// ExtractDomain extracts clean hostname from a URL.
func ExtractDomain(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	host = strings.TrimPrefix(host, "www.")
	return host
}

// PageContainsOrgnr checks whether page text contains the 9-digit orgnr
// with or without spacing (e.g. "923609016" or "923 609 016").
func PageContainsOrgnr(pageText, orgnr string) bool {
	if len(orgnr) != 9 {
		return false
	}
	if strings.Contains(pageText, orgnr) {
		return true
	}
	// Also test 3-3-3 spaced format: "923 609 016"
	spaced := orgnr[0:3] + " " + orgnr[3:6] + " " + orgnr[6:9]
	if strings.Contains(pageText, spaced) {
		return true
	}
	// Also test with non-breaking spaces or dots: "923.609.016"
	dotted := orgnr[0:3] + "." + orgnr[3:6] + "." + orgnr[6:9]
	return strings.Contains(pageText, dotted)
}

// VerifyPage evaluates whether a fetched web page conclusively belongs to the target company.
// Rules (from Section 6.6 of spec):
// 1. Accepted if page explicitly contains the 9-digit org number.
// 2. Accepted if page domain matches the registry website AND normalized company name appears.
// 3. Accepted if page domain matches registry website AND company name AND (postcode OR municipality) appear.
// 4. Name-only matches without registry domain or without location anchor are strictly rejected.
func VerifyPage(pageText, pageURL, registryWebsite, companyName, postcode, municipality, targetOrgnr string) (bool, string) {
	if strings.TrimSpace(pageText) == "" {
		return false, "empty page text"
	}

	// Rule 1: High confidence - orgnr on page
	if PageContainsOrgnr(pageText, targetOrgnr) {
		return true, "Page contains matching 9-digit organisation number"
	}

	// Compare domains
	pageDomain := ExtractDomain(pageURL)
	regDomain := ExtractDomain(registryWebsite)

	if regDomain == "" || pageDomain == "" || regDomain != pageDomain {
		return false, "Page domain does not match registry-listed official website"
	}

	normName := NormalizeName(companyName)
	normPage := strings.ToLower(pageText)

	hasName := normName != "" && len(normName) >= 3 && strings.Contains(normPage, normName)
	hasPostcode := postcode != "" && strings.Contains(normPage, postcode)
	hasMuni := municipality != "" && strings.Contains(normPage, strings.ToLower(municipality))

	// Rule 2 & 3: Domain matches + Name + location anchor
	if hasName && (hasPostcode || hasMuni) {
		return true, "Page domain matches registry website with confirmed company name and geographical location"
	}

	if hasName {
		return true, "Page domain matches official registry website with confirmed company name"
	}

	return false, "Page belongs to matching domain but lacks confirmed company name or identifier"
}

// VerifyLLMClaim verifies that an LLM extracted claim carries an exact verbatim snippet found in the accepted page text.
func VerifyLLMClaim(snippet, acceptedPageText string) error {
	cleanSnippet := strings.TrimSpace(snippet)
	if cleanSnippet == "" {
		return errors.New("empty evidence snippet")
	}
	if len(cleanSnippet) < 15 {
		return errors.New("snippet too short to establish verification")
	}

	// Check if snippet exists verbatim in accepted page text
	if !strings.Contains(acceptedPageText, cleanSnippet) {
		// Normalize whitespace and check again
		normSnippet := strings.Join(strings.Fields(cleanSnippet), " ")
		normPage := strings.Join(strings.Fields(acceptedPageText), " ")
		if !strings.Contains(normPage, normSnippet) {
			return ErrSnippetNotFound
		}
	}
	return nil
}

// ContainsDigits returns true if string contains any numeric character.
func ContainsDigits(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
