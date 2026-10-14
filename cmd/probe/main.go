package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"signalpost/internal/fetch"
)

func main() {
	client, err := fetch.NewClient(fetch.Options{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Client init err: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	orgnrs := []string{
		"923609016", // Equinor ASA
		"974760673", // Brønnøysundregistrene
		"984851006", // DNB Bank ASA
	}

	_ = os.MkdirAll("testdata", 0755)

	for _, nr := range orgnrs {
		// 1. Enhetsregisteret
		enhetURL := fmt.Sprintf("https://data.brreg.no/enhetsregisteret/api/enheter/%s", nr)
		fmt.Printf("Fetching enhet %s...\n", enhetURL)
		resp, err := client.Get(ctx, enhetURL, 24*time.Hour)
		if err != nil {
			fmt.Printf("Enhet error for %s: %v\n", nr, err)
		} else {
			fmt.Printf("Enhet status: %d, body size: %d bytes\n", resp.StatusCode, len(resp.Body))
			_ = os.WriteFile(filepath.Join("testdata", fmt.Sprintf("enhet_%s.json", nr)), resp.Body, 0644)
		}

		// 2. Regnskapsregisteret
		regnskapURL := fmt.Sprintf("https://data.brreg.no/regnskapsregisteret/regnskap/%s", nr)
		fmt.Printf("Fetching regnskap %s...\n", regnskapURL)
		rResp, err := client.Get(ctx, regnskapURL, 24*time.Hour)
		if err != nil {
			fmt.Printf("Regnskap error for %s: %v\n", nr, err)
		} else {
			fmt.Printf("Regnskap status: %d, body size: %d bytes\n", rResp.StatusCode, len(rResp.Body))
			_ = os.WriteFile(filepath.Join("testdata", fmt.Sprintf("regnskap_%s.json", nr)), rResp.Body, 0644)
		}
	}
}
