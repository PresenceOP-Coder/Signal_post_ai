package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"

	"signalpost/internal/diff"
	"signalpost/internal/explain"
	"signalpost/internal/fetch"
	"signalpost/internal/model"
	"signalpost/internal/orgnr"
	"signalpost/internal/sources/brreg"
	"signalpost/internal/sources/web"
	"signalpost/internal/store"
)

type Config struct {
	Concurrency int
	Client      *fetch.Client
	Store       *store.Store
	Report      *model.RunReport
}

type Pipeline struct {
	cfg Config
}

func NewPipeline(cfg Config) *Pipeline {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}
	return &Pipeline{cfg: cfg}
}

// ProcessCompany processes a single company end-to-end adhering to all validation and evidence rules.
func (p *Pipeline) ProcessCompany(ctx context.Context, rawInput string) (*model.Profile, error) {
	now := time.Now().UTC()

	// 1. Normalize and validate MOD11 checksum (zero network requests if invalid)
	cleanOrgnr, err := orgnr.CleanAndValidate(rawInput)
	if err != nil {
		prof := model.NewProfile(rawInput)
		prof.Status = "invalid_orgnr"
		prof.Errors = append(prof.Errors, fmt.Sprintf("MOD11 validation failed: %v", err))
		prof.Summary = fmt.Sprintf("Invalid organisation number %q: %v", rawInput, err)
		if p.cfg.Store != nil {
			_ = p.cfg.Store.SaveProfile(prof)
		}
		if p.cfg.Report != nil {
			p.cfg.Report.RecordError(rawInput, err.Error())
		}
		return prof, err
	}

	// 2. Load prior profile (if present in primary store or seed store)
	var prior *model.Profile
	if p.cfg.Store != nil {
		prior, _ = p.cfg.Store.LoadProfile(cleanOrgnr)
	}

	// 3. Fetch registry entity facts
	var allFacts []model.Fact
	entityFacts, err := brreg.FetchEntity(ctx, p.cfg.Client, cleanOrgnr)
	if err != nil {
		prof := model.NewProfile(cleanOrgnr)
		prof.Status = "fetch_failed"
		prof.Errors = append(prof.Errors, fmt.Sprintf("Registry fetch failed: %v", err))
		prof.Summary = fmt.Sprintf("Failed to fetch official registration: %v", err)
		if p.cfg.Store != nil {
			_ = p.cfg.Store.SaveProfile(prof)
		}
		if p.cfg.Report != nil {
			p.cfg.Report.RecordError(cleanOrgnr, err.Error())
		}
		return prof, err
	}
	allFacts = append(allFacts, entityFacts...)

	// 4. Fetch official annual accounts (if commercial entity)
	accFacts, err := brreg.FetchAccounts(ctx, p.cfg.Client, cleanOrgnr)
	if err == nil && len(accFacts) > 0 {
		allFacts = append(allFacts, accFacts...)
	}

	// 5. Fetch official governance roles (chair, general manager, auditor)
	rolesFacts, err := brreg.FetchRoles(ctx, p.cfg.Client, cleanOrgnr)
	if err == nil && len(rolesFacts) > 0 {
		allFacts = append(allFacts, rolesFacts...)
	}

	// 6. Verified website heuristic enrichment (description, social links, contact, careers)
	var websiteURL, legalName, muni string
	for _, f := range allFacts {
		if f.Key == "website" {
			if s, ok := f.Value.(string); ok {
				websiteURL = s
			}
		}
		if f.Key == "legal_name" {
			if s, ok := f.Value.(string); ok {
				legalName = s
			}
		}
		if f.Key == "municipality" {
			if s, ok := f.Value.(string); ok {
				muni = s
			}
		}
	}
	if websiteURL != "" {
		webFacts, err := web.ScrapeWebsite(ctx, p.cfg.Client, websiteURL, legalName, "", muni, cleanOrgnr)
		if err == nil && len(webFacts) > 0 {
			allFacts = append(allFacts, webFacts...)
		}
	}

	// 7. Diff against prior profile and track changes
	currentProfile := diff.DiffAndApply(prior, cleanOrgnr, allFacts, now)

	// 6. Generate explanations (summary, changes, gaps)
	explain.GenerateExplanations(currentProfile)

	// 7. Save to profile store
	if p.cfg.Store != nil {
		if err := p.cfg.Store.SaveProfile(currentProfile); err != nil {
			return currentProfile, fmt.Errorf("failed to save profile %s: %w", cleanOrgnr, err)
		}
	}

	// 8. Update metrics and coverage
	if p.cfg.Report != nil {
		p.cfg.Report.RecordSuccess()
		for _, f := range currentProfile.Facts {
			p.cfg.Report.Coverage.TotalFactsRecorded++
			p.cfg.Report.Coverage.KeyDistribution[f.Key]++
		}
	}

	return currentProfile, nil
}

// Run processes an array of organisation numbers concurrently through the worker pool.
func (p *Pipeline) Run(ctx context.Context, orgnrs []string) ([]*model.Profile, error) {
	if p.cfg.Report != nil {
		p.cfg.Report.CompaniesTotal = len(orgnrs)
	}

	jobs := make(chan string, len(orgnrs))
	results := make(chan *model.Profile, len(orgnrs))

	var wg sync.WaitGroup

	// Launch worker pool
	for i := 0; i < p.cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for nr := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				prof, _ := p.ProcessCompany(ctx, nr)
				results <- prof
			}
		}()
	}

	for _, nr := range orgnrs {
		jobs <- nr
	}
	close(jobs)

	wg.Wait()
	close(results)

	profiles := make([]*model.Profile, 0, len(orgnrs))
	for prof := range results {
		profiles = append(profiles, prof)
	}

	return profiles, nil
}
