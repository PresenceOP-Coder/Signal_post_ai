package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"signalpost/internal/fetch"
	"signalpost/internal/orgnr"
)

type enhetSearchResponse struct {
	Embedded struct {
		Enheter []struct {
			Organisasjonsnummer string `json:"organisasjonsnummer"`
			Navn                string `json:"navn"`
			Konkurs             bool   `json:"konkurs"`
			Slettedato          string `json:"slettedato"`
		} `json:"enheter"`
	} `json:"_embedded"`
	Page struct {
		Size          int `json:"size"`
		TotalElements int `json:"totalElements"`
		TotalPages    int `json:"totalPages"`
		Number        int `json:"number"`
	} `json:"page"`
}

func main() {
	count := flag.Int("count", 1000, "Number of active organisation numbers to sample")
	outFile := flag.String("out", "orgnrs_1000.txt", "Output file path")
	flag.Parse()

	client, err := fetch.NewClient(fetch.Options{
		MaxRequests: 500,
		Timeout:     10 * time.Second,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Client init err: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	collected := make([]string, 0, *count)
	seen := make(map[string]bool)

	pageSize := 100
	page := 0

	fmt.Printf("Sampling %d active organisation numbers from Enhetsregisteret...\n", *count)

	for len(collected) < *count {
		searchURL := fmt.Sprintf("https://data.brreg.no/enhetsregisteret/api/enheter?page=%d&size=%d", page, pageSize)
		resp, err := client.Get(ctx, searchURL, 24*time.Hour)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Fetch error on page %d: %v\n", page, err)
			break
		}

		if resp.StatusCode != 200 {
			fmt.Fprintf(os.Stderr, "Unexpected status %d on page %d\n", resp.StatusCode, page)
			break
		}

		var searchResp enhetSearchResponse
		if err := json.Unmarshal(resp.Body, &searchResp); err != nil {
			fmt.Fprintf(os.Stderr, "JSON decode error on page %d: %v\n", page, err)
			break
		}

		enheter := searchResp.Embedded.Enheter
		if len(enheter) == 0 {
			break
		}

		for _, e := range enheter {
			nr := e.Organisasjonsnummer
			if e.Konkurs || e.Slettedato != "" {
				continue
			}
			if err := orgnr.Validate(nr); err == nil && !seen[nr] {
				seen[nr] = true
				collected = append(collected, nr)
				if len(collected) >= *count {
					break
				}
			}
		}

		page++
		if page%5 == 0 {
			fmt.Printf("Collected %d / %d org numbers...\n", len(collected), *count)
		}
	}

	fmt.Printf("Total collected: %d valid active organisation numbers.\n", len(collected))

	f, err := os.Create(*outFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create %s: %v\n", *outFile, err)
		os.Exit(1)
	}
	defer f.Close()

	for _, nr := range collected {
		_, _ = fmt.Fprintln(f, nr)
	}

	fmt.Printf("Saved to %s successfully.\n", *outFile)
}
