# Signalpost Agent

Signalpost is a deterministic CLI agent written in Go that ingests Norwegian organisation numbers (9 digits) and builds comprehensive, verified company profiles from official registries.

Every fact is anchored with an exact `source_url`, a `retrieved_at` timestamp, an `origin` tag, and a 1-line plain-language `evidence` trail. It also tracks historical modifications between runs and generates deterministic summaries, change logs, and data gap explanations without LLM hallucinations.

---

## The Philosophy: Why Deterministic First?

In corporate intelligence, guessing a number or attributing facts to the wrong entity isn't just inaccurate—under the Builderr scoring rules, **a fabricated financial value or wrong-company match causes immediate disqualification**.

Many company names in Norway collide (e.g. dozens of businesses share common family or geographical names). Letting an LLM guess financial numbers or scrape unverified web search results is a liability.

Signalpost takes a strict stance:
1. **Never let an LLM invent numbers.** Financial data is pulled strictly from official statutory filings (*Regnskapsregisteret*).
2. **Never publish an unverified fact.** Web discovery only accepts pages that explicitly match the 9-digit organisation number or share the registry-verified domain with matching geographical/name anchors.
3. **Always explain the trail.** If a data point exists, the profile cites the exact public endpoint and timestamp. If a company lacks accounts or a website, the profile clearly explains *why* (e.g. non-commercial entity, holding company, or exempt form).

---

## Quickstart

### Prerequisites
- **Go 1.25+** (tested up to Go 1.26)
- Git

### One-Command Run
```bash
# Process the input organisation numbers and output profiles
go run ./cmd/signalpost/main.go run --input orgnrs.txt --out profiles/
```
*(Or use `make run`)*

### Run the Audit & Scorer
```bash
# Audits all invariants, measures field coverage, and estimates hackathon points
go run ./cmd/scorer/main.go --profiles profiles --report profiles/run_report.json
```
*(Or use `make score`)*

### Run the Test Suite
```bash
go test -v ./...
```

---

## How It Works (Pipeline Architecture)

```
                       +-----------------------+
  orgnrs.txt  ------>  |  CLI (cmd/signalpost) |  flags: --input --out --seed --max-requests --concurrency
                       +-----------+-----------+
                                   |
                          +--------v---------+
                          |   Worker Pool    | (Default: 10-16 goroutines)
                          +--------+---------+
                                   |  (per company)
    +---------------+--------------+----+---------------+-----------------+
    |               |                   |               |                 |
[1. Validate]  [2. Enhetsregisteret]  [3. Accounts]  [4. Roles]     [5. Web Heuristics]
 MOD11 check    status, name, NACE,    balance sheet  daglig leder,  domain matching,
 zero requests  address, VAT, capital  & multi-year   styreleder,    orgnr verification,
 if invalid     & employee count       financials     auditor        social/career links
    |               |                   |               |                 |
    +---------------+-------------------+---------------+-----------------+
                                        |
                             [6. Fact Verifier & Store]
                             (Enforces origins & strict evidence)
                                        |
                             [7. Differ & History] <--- prior profile / seed/
                             (Invariant 7: 0 false diffs)
                                        |
                             [8. Templated Explainer]
                             (Summary, change logs, data gaps)
                                        |
                             [9. Output Writer]
                             profiles/{orgnr}.json + profiles.jsonl + run_report.json
```

1. **Pre-flight Check (`internal/orgnr`):** Strips formatting (`NO`, `MVA`, spaces) and runs official MOD-11 checksum weights `[3, 2, 7, 6, 5, 4, 3, 2]`. Invalid numbers are flagged immediately with zero network overhead.
2. **Entity Registration (`internal/sources/brreg/entity.go`):** Queries Enhetsregisteret for status (`active`, `bankrupt`, `under_liquidation`, `deleted`), statutory corporate purpose, registered share capital, institutional sector, NACE industry classification, and registered employees.
3. **Statutory Accounts (`internal/sources/brreg/accounts.go`):** Queries Regnskapsregisteret for multi-year financial statements (`revenue`, `operating_result`, `net_result`, `total_assets`, `equity`, `fiscal_year`). Prioritizes company-level (*SELSKAP*) filings over consolidated group (*KONSERN*) accounts where available.
4. **Governance Roles (`internal/sources/brreg/roles.go`):** Extracts official office holders (Daglig leder / General Manager, Styrets leder / Board Chair, and Revisor / Auditor).
5. **Verified Website Scraping (`internal/sources/web/scraper.go`):** Fetches the homepage and extracts meta descriptions, career pages, contact URLs, and social profiles. **Rejected** if the page fails the 3-layer anti-collision acceptance rule.
6. **Fact Diff & History (`internal/diff/differ.go`):** Compares newly gathered facts against the prior baseline or `seed/` store. 
   - **Invariant 7:** If no facts changed, only `last_verified` updates; `last_changed` and `history` stay untouched.
7. **Templated Explanations (`internal/explain/explainer.go`):** Assembles human-readable summaries and explanations for missing fields (e.g. non-commercial entities exempt from filing financial statements).

---

## Output Format

Each profile is saved as `profiles/{orgnr}.json`. An aggregate line-delimited `profiles.jsonl` is also generated for downstream ingestion.

```json
{
  "orgnr": "923609016",
  "legal_name": "EQUINOR ASA",
  "status": "active",
  "facts": {
    "legal_name": {
      "key": "legal_name",
      "value": "EQUINOR ASA",
      "source_url": "https://data.brreg.no/enhetsregisteret/api/enheter/923609016",
      "retrieved_at": "2026-10-14T19:30:15Z",
      "origin": "registry",
      "evidence": "Official registered legal name in Enhetsregisteret for 923609016"
    },
    "revenue:2024": {
      "key": "revenue",
      "value": 103774000000,
      "unit": "USD",
      "period": "2024",
      "source_url": "https://data.brreg.no/regnskapsregisteret/regnskap/923609016",
      "retrieved_at": "2026-10-14T19:30:16Z",
      "origin": "accounts",
      "evidence": "Regnskapsregisteret official SELSKAP annual accounts for 923609016 (FY2024)"
    }
  },
  "history": [],
  "summary": "Active ASA in STAVANGER, NACE 06.100, 21272 employees. Annual accounts for FY2025 on file.",
  "changes_summary": [
    "Baseline profile established; verified against official registries."
  ],
  "gaps": [
    "Email address not published in registry record."
  ],
  "last_verified": "2026-10-14T19:35:00Z",
  "last_changed": "2026-10-14T19:30:15Z",
  "created_at": "2026-10-14T19:30:15Z"
}
```

---

## Performance & Daily Limits Benchmark

Signalpost was engineered to operate with plenty of headroom below the Builderr daily limits:

| Requirement | Hackathon Limit | Signalpost 100-Company Test | Headroom |
| :--- | :--- | :--- | :--- |
| **Run Duration** | < 45 minutes | **~4.5 seconds** | > 99% faster |
| **Outbound Requests** | < 2,000 requests | **6 requests** (with disk cache) / ~200 fresh | Well below cap |
| **External API Costs** | < $10.00 | **$0.00** | Free public registries |
| **Pre-generated Dataset** | ≥ 1,000 profiles | **1,005 verified profiles** | Fulfilled |
| **Invariant Failures** | 0 allowed | **0 failures** | 100% compliance |

---

## CLI Reference & Flags

```bash
# Process a custom file of organisation numbers
go run ./cmd/signalpost/main.go run --input my_companies.txt --out output/

# Available flags:
#   --input        Path to file with 9-digit org numbers (default: orgnrs.txt)
#   --out          Directory to write profiles and run_report.json (default: profiles/)
#   --store        Directory storing prior profiles for diffing (defaults to --out)
#   --seed         Directory storing baseline seed profiles (default: seed/)
#   --max-requests Hard cap on outbound requests before stopping (default: 1800)
#   --concurrency  Worker goroutine pool size (default: 10)
#   --no-llm       Run in pure deterministic mode without LLM calls (default: true)
```

### Self-Scorer Tool
The built-in audit scorer inspects every generated profile, validates the 8 strict invariants, checks catalogue coverage, and prints a score breakdown:
```bash
go run ./cmd/scorer/main.go --profiles profiles/ --report profiles/run_report.json
```

### Sampling Active Companies
To generate a new batch of active Norwegian companies directly from the Brreg search API:
```bash
go run ./cmd/sample/main.go -count 1000 -out orgnrs_1000.txt
```

---

## Repository Structure

```
cmd/
  signalpost/main.go     # Main CLI entrypoint
  scorer/main.go         # Self-scoring audit tool for hackathon metrics
  sample/main.go         # Active Norwegian org number sampler
internal/
  model/                 # Fact, Profile, Change, RunReport models & invariant logic
  orgnr/                 # 9-digit normalization & MOD11 checksum algorithm
  fetch/                 # Cached, token-bucket rate-limited HTTP client with hard cap
  sources/brreg/         # Enhetsregisteret, Regnskapsregisteret & Roles adapters
  sources/web/           # Verified domain discovery & heuristic web scraper
  verify/                # Same-company acceptance & anti-collision rules
  diff/                  # Fact-level diffing & history tracking
  explain/               # Templated summaries, change logs & gap detection
  pipeline/              # Concurrent worker pool orchestrator
  store/                 # Profile persistence & seed loader
testdata/                # Live official registry JSON fixtures
profiles/                # Pre-generated 1,005 company profiles dataset
seed/                    # Baseline store for diff evaluation
Makefile                 # Standard build and run targets
.env.example             # Configuration reference
```

---

## License

Created for the Builderr.ai / Unstop Hackathon 2026. Available under the MIT License.
