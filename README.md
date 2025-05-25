# Group Scholar Pacing Console

Group Scholar Pacing Console is a Go-based TUI that monitors award disbursement pacing, highlights behind-schedule awards, and keeps check-in cadence visible for program ops.

## Features
- Award pacing status derived from disbursed vs expected progress
- Summary header with awarded/disbursed/expected totals, gap amounts, pace mix, and check-in risk counts
- Check-in urgency signals (overdue / due soon / upcoming)
- Insights panel with owner pulse, cohort watchlist, and status mix
- TUI list with filter support and detail panel
- Priority sort plus quick focus filter for risk items
- Sample disbursement dataset for quick demos
- Shareable pacing reports in text or JSON
- Trend reports comparing the latest two Postgres snapshots

## Getting started

```bash
go mod tidy

go run .
```

Run with a custom dataset:

```bash
go run . -data path/to/disbursements.json
```

Load the latest snapshot from Postgres:

```bash
go run . -source db -db-url "$PACECONSOLE_DATABASE_URL"
```

Export the current snapshot to CSV or JSON (defaults to CSV if no extension):

```bash
go run . -export pacing-snapshot.csv
go run . -export pacing-snapshot.json -export-filter risk
```

Exports include expected disbursement amounts and gap deltas for each award.

Generate a pacing report (text default, JSON supported; use `-` for stdout):

```bash
go run . -report pacing-report.txt
go run . -report pacing-report.json
go run . -report - -report-format text
```

Generate a trend report from the latest two Postgres snapshots:

```bash
go run . -trend-report pacing-trend.txt -db-url "$PACECONSOLE_DATABASE_URL"
go run . -trend-report - -trend-format json -db-url "$PACECONSOLE_DATABASE_URL"
```

Write a fresh snapshot to Postgres (production only):

```bash
go run . -db-sync -db-url "$PACECONSOLE_DATABASE_URL"
```

Adjust the due-soon window for check-ins (default 14 days):

```bash
go run . -checkin-window 10
```

Filter the dataset before loading the console (comma-separated, case-insensitive):

```bash
go run . -owner "Maya R.,Jordan P."
go run . -cohort "Spring 2025" -status "Active,Unspecified"
```

## Data format

```json
[
  {
    "scholar": "Avery Nguyen",
    "cohort": "Spring 2025",
    "amount": 12000,
    "disbursed_to_date": 7800,
    "award_date": "2025-02-15",
    "target_date": "2026-02-15",
    "next_checkin": "2026-02-20",
    "owner": "Maya R.",
    "status": "Active",
    "notes": "On track with tuition schedule."
  }
]
```

## Controls
- `/` to filter
- `s` to toggle sort mode (priority vs alpha)
- `f` to toggle focus mode (all vs risk)
- `i` to toggle the insights panel
- `r` to refresh the timestamp
- `q` to quit

## Tech
- Go
- Bubble Tea + Lip Gloss
- Postgres (optional sync + snapshot storage)
- Postgres (optional data source)
