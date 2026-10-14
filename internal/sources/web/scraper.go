package web

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"signalpost/internal/fetch"
	"signalpost/internal/model"
	"signalpost/internal/verify"
)

var (
	scriptTagRegex   = regexp.MustCompile(`(?is)<script.*?>.*?</script>`)
	styleTagRegex    = regexp.MustCompile(`(?is)<style.*?>.*?</style>`)
	htmlTagRegex     = regexp.MustCompile(`<[^>]+>`)
	metaDescRegex    = regexp.MustCompile(`(?is)<meta[^>]+(?:name|property)=["'](?:description|og:description)["'][^>]+content=["']([^"']+)["']`)
	metaDescAltRegex = regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']+)["'][^>]+(?:name|property)=["'](?:description|og:description)["']`)
	hrefRegex        = regexp.MustCompile(`(?is)href=["']([^"']+)["']`)
)

func stripHTML(rawHTML string) string {
	s := scriptTagRegex.ReplaceAllString(rawHTML, " ")
	s = styleTagRegex.ReplaceAllString(s, " ")
	s = htmlTagRegex.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	// Collapse whitespace
	return strings.Join(strings.Fields(s), " ")
}

// ScrapeWebsite performs verified heuristic extraction from a company's official website.
func ScrapeWebsite(ctx context.Context, client *fetch.Client, websiteURL, companyName, postcode, municipality, orgnr string) ([]model.Fact, error) {
	cleanURL := strings.TrimSpace(websiteURL)
	if cleanURL == "" {
		return nil, nil
	}
	if !strings.HasPrefix(cleanURL, "http://") && !strings.HasPrefix(cleanURL, "https://") {
		cleanURL = "https://" + cleanURL
	}

	resp, err := client.Get(ctx, cleanURL, 7*24*time.Hour)
	if err != nil || resp.StatusCode != 200 || len(resp.Body) == 0 {
		return nil, nil
	}

	rawHTML := string(resp.Body)
	plainText := stripHTML(rawHTML)

	// Invariant: Verify page acceptance according to same-company rules
	accepted, reason := verify.VerifyPage(plainText, resp.FinalURL, cleanURL, companyName, postcode, municipality, orgnr)
	if !accepted {
		return nil, nil // Silently discard unverified websites
	}

	var facts []model.Fact
	srcURL := resp.FinalURL
	if srcURL == "" {
		srcURL = cleanURL
	}
	evidence := fmt.Sprintf("Extracted from verified company website (%s)", reason)

	// 1. Meta Description
	var description string
	if m := metaDescRegex.FindStringSubmatch(rawHTML); len(m) > 1 {
		description = html.UnescapeString(strings.TrimSpace(m[1]))
	} else if m := metaDescAltRegex.FindStringSubmatch(rawHTML); len(m) > 1 {
		description = html.UnescapeString(strings.TrimSpace(m[1]))
	}

	if description != "" && len(description) >= 15 {
		if f, err := model.NewFact("description", description, "", "", srcURL, resp.FetchedAt, model.OriginPage, evidence); err == nil {
			facts = append(facts, f)
		}
	}

	// 2. Discover links (social, contact, careers)
	parsedBase, _ := url.Parse(srcURL)
	matches := hrefRegex.FindAllStringSubmatch(rawHTML, -1)

	var socialLinks []string
	seenSocial := make(map[string]bool)
	var contactPage string
	var careersPage string

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		rawHref := strings.TrimSpace(m[1])
		if rawHref == "" || strings.HasPrefix(rawHref, "#") || strings.HasPrefix(rawHref, "javascript:") {
			continue
		}

		lowerHref := strings.ToLower(rawHref)

		// Social links
		if (strings.Contains(lowerHref, "linkedin.com/company") ||
			strings.Contains(lowerHref, "facebook.com/") ||
			strings.Contains(lowerHref, "twitter.com/") ||
			strings.Contains(lowerHref, "x.com/") ||
			strings.Contains(lowerHref, "instagram.com/") ||
			strings.Contains(lowerHref, "youtube.com/")) && !seenSocial[rawHref] {
			seenSocial[rawHref] = true
			socialLinks = append(socialLinks, rawHref)
		}

		// Resolve relative link
		resolved := rawHref
		if parsedBase != nil {
			if u, err := parsedBase.Parse(rawHref); err == nil {
				resolved = u.String()
			}
		}

		// Contact page
		if contactPage == "" && (strings.Contains(lowerHref, "kontakt") || strings.Contains(lowerHref, "contact")) {
			contactPage = resolved
		}

		// Careers page
		if careersPage == "" && (strings.Contains(lowerHref, "karriere") || strings.Contains(lowerHref, "careers") || strings.Contains(lowerHref, "jobb") || strings.Contains(lowerHref, "stillinger")) {
			careersPage = resolved
		}
	}

	if len(socialLinks) > 0 {
		if f, err := model.NewFact("social_links", strings.Join(socialLinks, ", "), "", "", srcURL, resp.FetchedAt, model.OriginPage, evidence); err == nil {
			facts = append(facts, f)
		}
	}
	if contactPage != "" {
		if f, err := model.NewFact("contact_page", contactPage, "", "", srcURL, resp.FetchedAt, model.OriginPage, evidence); err == nil {
			facts = append(facts, f)
		}
	}
	if careersPage != "" {
		if f, err := model.NewFact("careers_page", careersPage, "", "", srcURL, resp.FetchedAt, model.OriginPage, evidence); err == nil {
			facts = append(facts, f)
		}
	}

	return facts, nil
}
