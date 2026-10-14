package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"signalpost/internal/fetch"
	"signalpost/internal/model"
	"signalpost/internal/pipeline"
	"signalpost/internal/store"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		runCmd(os.Args[2:])
		return
	}

	// Default to runCmd if no subcommand provided
	runCmd(os.Args[1:])
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)

	inputFile := fs.String("input", "orgnrs.txt", "Path to file containing Norwegian organisation numbers")
	outDir := fs.String("out", "profiles", "Directory to write individual company profiles and run_report.json")
	storeDir := fs.String("store", "", "Directory storing prior profiles for diffing (defaults to --out)")
	seedDir := fs.String("seed", "seed", "Directory storing repository seed profiles")
	maxRequests := fs.Int64("max-requests", 1800, "Hard cap on total outbound HTTP requests")
	concurrency := fs.Int("concurrency", 10, "Number of concurrent worker goroutines")
	noLLM := fs.Bool("no-llm", true, "Disable LLM enrichment")

	_ = fs.Parse(args)
	_ = noLLM

	if *storeDir == "" {
		*storeDir = *outDir
	}

	fmt.Printf("=== Signalpost Agent ===\n")
	fmt.Printf("Input file:      %s\n", *inputFile)
	fmt.Printf("Output dir:      %s\n", *outDir)
	fmt.Printf("Store dir:       %s\n", *storeDir)
	fmt.Printf("Seed dir:        %s\n", *seedDir)
	fmt.Printf("Request cap:     %d\n", *maxRequests)
	fmt.Printf("Concurrency:     %d\n", *concurrency)
	fmt.Printf("========================\n\n")

	// Read input organisation numbers
	orgnrs, err := readOrgnrs(*inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input file %q: %v\n", *inputFile, err)
		os.Exit(1)
	}

	if len(orgnrs) == 0 {
		fmt.Printf("No organisation numbers found in %s. Exiting.\n", *inputFile)
		return
	}

	fmt.Printf("Loaded %d organisation numbers to process.\n", len(orgnrs))

	// Initialize Store
	profStore, err := store.NewStore(*outDir, *seedDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize store: %v\n", err)
		os.Exit(1)
	}

	// Initialize Run Report
	report := model.NewRunReport()

	// Initialize Fetch Client
	client, err := fetch.NewClient(fetch.Options{
		MaxRequests: *maxRequests,
		Timeout:     10 * time.Second,
		Report:      report,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize fetch client: %v\n", err)
		os.Exit(1)
	}

	// Initialize Pipeline
	pipe := pipeline.NewPipeline(pipeline.Config{
		Concurrency: *concurrency,
		Client:      client,
		Store:       profStore,
		Report:      report,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()

	startTime := time.Now()
	profiles, err := pipe.Run(ctx, orgnrs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Pipeline execution error: %v\n", err)
	}

	// Save aggregate profiles.jsonl
	jsonlPath := filepath.Join(*outDir, "profiles.jsonl")
	if err := profStore.WriteJSONL(jsonlPath, profiles); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write %s: %v\n", jsonlPath, err)
	} else {
		fmt.Printf("Wrote aggregate %s\n", jsonlPath)
	}

	// Finalize and save run_report.json
	report.Finalize()
	reportPath := filepath.Join(*outDir, "run_report.json")
	reportData, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		_ = os.WriteFile(reportPath, reportData, 0644)
		fmt.Printf("Wrote run report %s\n", reportPath)
	}

	elapsed := time.Since(startTime)
	fmt.Printf("\n=== Execution Summary ===\n")
	fmt.Printf("Total processed:   %d\n", report.CompaniesTotal)
	fmt.Printf("Successful:        %d\n", report.CompaniesSuccessful)
	fmt.Printf("Failed:            %d\n", report.CompaniesFailed)
	fmt.Printf("Total requests:    %d / %d (Cap)\n", report.TotalRequests, *maxRequests)
	fmt.Printf("Cache hits:        %d\n", report.CacheHits)
	fmt.Printf("Facts recorded:    %d (Avg %.1f / company)\n", report.Coverage.TotalFactsRecorded, report.Coverage.FactsPerCompanyAvg)
	fmt.Printf("Time elapsed:      %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("=========================\n")
}

func readOrgnrs(filePath string) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines, scanner.Err()
}
