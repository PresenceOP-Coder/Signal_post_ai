package model

import (
	"sync"
	"time"
)

// CoverageStats tracks fact coverage metrics across the run.
type CoverageStats struct {
	TotalFactsRecorded int            `json:"total_facts_recorded"`
	FactsPerCompanyAvg float64        `json:"facts_per_company_avg"`
	KeyDistribution    map[string]int `json:"key_distribution"`
}

// RunReport stores runtime execution metrics for auditing compliance against hackathon limits.
type RunReport struct {
	mu                  sync.Mutex
	StartTime           time.Time         `json:"start_time"`
	EndTime             time.Time         `json:"end_time"`
	DurationSeconds     float64           `json:"duration_seconds"`
	CompaniesTotal      int               `json:"companies_total"`
	CompaniesSuccessful int               `json:"companies_successful"`
	CompaniesFailed     int               `json:"companies_failed"`
	TotalRequests       int64             `json:"total_requests"`
	RequestsPerHost     map[string]int64  `json:"requests_per_host"`
	CacheHits           int64             `json:"cache_hits"`
	HardCapReached      bool              `json:"hard_cap_reached"`
	LLMTokensInput      int64             `json:"llm_tokens_input"`
	LLMTokensOutput     int64             `json:"llm_tokens_output"`
	EstimatedLLMCostUSD float64           `json:"estimated_llm_cost_usd"`
	Errors              map[string]string `json:"errors,omitempty"` // orgnr -> error description
	Coverage            CoverageStats     `json:"coverage"`
}

func NewRunReport() *RunReport {
	return &RunReport{
		StartTime:       time.Now().UTC(),
		RequestsPerHost: make(map[string]int64),
		Errors:          make(map[string]string),
		Coverage: CoverageStats{
			KeyDistribution: make(map[string]int),
		},
	}
}

func (r *RunReport) RecordRequest(host string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.TotalRequests++
	r.RequestsPerHost[host]++
}

func (r *RunReport) RecordCacheHit() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.CacheHits++
}

func (r *RunReport) RecordError(orgnr, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.CompaniesFailed++
	r.Errors[orgnr] = msg
}

func (r *RunReport) RecordSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.CompaniesSuccessful++
}

func (r *RunReport) Finalize() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.EndTime = time.Now().UTC()
	r.DurationSeconds = r.EndTime.Sub(r.StartTime).Seconds()
	if r.CompaniesTotal > 0 {
		r.Coverage.FactsPerCompanyAvg = float64(r.Coverage.TotalFactsRecorded) / float64(r.CompaniesTotal)
	}
}
