# Group Scholar Pacing Console

Group Scholar Pacing Console is a Go-based TUI that monitors award disbursement pacing, highlights behind-schedule awards, and keeps check-in cadence visible for program ops.

## Features
- Award pacing status derived from disbursed vs expected progress
- Summary header with awarded/disbursed totals, pace mix, and check-in risk counts
- Check-in urgency signals (overdue / due soon / upcoming)
- TUI list with filter support and detail panel
- Risk sort to surface behind-pace awards
- Sample disbursement dataset for quick demos

## Getting started

```bash
go mod tidy

go run .
```

Run with a custom dataset:

```bash
go run . -data path/to/disbursements.json
```

Adjust the due-soon window for check-ins (default 7 days):

```bash
go run . -due-soon 10
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
- `s` to toggle sort mode (default vs risk)
- `r` to refresh the timestamp
- `q` to quit

## Tech
- Go
- Bubble Tea + Lip Gloss
