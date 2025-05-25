package main

import (
	"strings"
	"testing"
	"time"
)

func TestCalculatePaceBehind(t *testing.T) {
	record := Disbursement{
		Amount:          1000,
		DisbursedToDate: 200,
		AwardDate:       "2025-01-01",
		TargetDate:      "2026-01-01",
	}
	now := time.Date(2025, 7, 2, 0, 0, 0, 0, time.UTC)
	pace := calculatePace(record, now)
	if pace.Label != "Behind" {
		t.Fatalf("expected Behind, got %s", pace.Label)
	}
	if pace.ExpectedAmount <= 0 {
		t.Fatalf("expected positive expected amount, got %0.2f", pace.ExpectedAmount)
	}
}

func TestCalculateRiskHigh(t *testing.T) {
	pace := paceStatus{Label: "Behind"}
	check := checkinStatus{Label: "Overdue"}
	risk := calculateRisk(pace, check)
	if risk.Level != "High" {
		t.Fatalf("expected High risk, got %s", risk.Level)
	}
	if risk.Score < 3 {
		t.Fatalf("expected risk score >= 3, got %d", risk.Score)
	}
}

func TestBuildInsightsIncludesOwners(t *testing.T) {
	records := []Disbursement{
		{
			Scholar:         "Avery",
			Cohort:          "Spring 2025",
			Owner:           "Maya R.",
			Status:          "Active",
			Amount:          10000,
			DisbursedToDate: 2000,
			AwardDate:       "2025-01-01",
			TargetDate:      "2025-12-31",
			NextCheckin:     "2025-03-01",
		},
		{
			Scholar:         "Riley",
			Cohort:          "Spring 2025",
			Owner:           "Jordan P.",
			Status:          "Active",
			Amount:          12000,
			DisbursedToDate: 8000,
			AwardDate:       "2025-01-01",
			TargetDate:      "2025-12-31",
			NextCheckin:     "2025-08-01",
		},
	}
	now := time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)
	items := buildItems(records, now, 14)
	insights := buildInsights(items)
	if !strings.Contains(insights, "Owner pulse") {
		t.Fatalf("expected owner pulse section")
	}
	if !strings.Contains(insights, "Maya R.") {
		t.Fatalf("expected owner name in insights")
	}
}

func TestApplyRecordFilters(t *testing.T) {
	records := []Disbursement{
		{Scholar: "A", Cohort: "Spring 2025", Owner: "Maya R.", Status: "Active"},
		{Scholar: "B", Cohort: "Fall 2025", Owner: "Jordan P.", Status: ""},
		{Scholar: "C", Cohort: "Spring 2025", Owner: "Maya R.", Status: "Paused"},
	}
	filters := parseRecordFilters("maya r.", "spring 2025", "active,unspecified")
	filtered := applyRecordFilters(records, filters)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 record, got %d", len(filtered))
	}
	if filtered[0].Scholar != "A" {
		t.Fatalf("expected scholar A, got %s", filtered[0].Scholar)
	}
}

func TestNormalizeReportFormatFromExtension(t *testing.T) {
	format, err := normalizeReportFormat("report.json", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if format != "json" {
		t.Fatalf("expected json format, got %s", format)
	}

	format, err = normalizeReportFormat("report.txt", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if format != "text" {
		t.Fatalf("expected text format, got %s", format)
	}
}

func TestBuildReportTextIncludesSummary(t *testing.T) {
	items := []awardItem{
		{
			data: Disbursement{
				Scholar:         "Avery",
				Cohort:          "Spring 2025",
				Owner:           "Maya R.",
				Status:          "Active",
				Amount:          10000,
				DisbursedToDate: 4000,
				AwardDate:       "2025-01-01",
				TargetDate:      "2025-12-31",
				NextCheckin:     "2025-05-01",
			},
			pace:  paceStatus{Label: "Behind", GapAmount: -1000},
			check: checkinStatus{Label: "Due Soon"},
			risk:  riskStatus{Level: "High"},
		},
	}
	metrics := calculateSummaryMetrics(items)
	report := buildReportText(items, metrics, time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC), 14)
	if !strings.Contains(report, "Group Scholar Pacing Report") {
		t.Fatalf("expected report title")
	}
	if !strings.Contains(report, "Owner pulse:") {
		t.Fatalf("expected owner pulse section")
	}
	if !strings.Contains(report, "Status mix:") {
		t.Fatalf("expected status mix section")
	}
}

func TestBuildTrendReportTextIncludesDelta(t *testing.T) {
	current := snapshotStats{
		GeneratedAt:    time.Date(2025, 5, 1, 9, 0, 0, 0, time.UTC),
		RecordCount:    10,
		TotalAwarded:   120000,
		TotalDisbursed: 70000,
		Ahead:          3,
		OnTrack:        4,
		Behind:         3,
		Overdue:        2,
		DueSoon:        1,
		High:           2,
		Medium:         4,
		Low:            4,
		DueSoonWindow:  14,
	}
	previous := snapshotStats{
		GeneratedAt:    time.Date(2025, 4, 1, 9, 0, 0, 0, time.UTC),
		RecordCount:    9,
		TotalAwarded:   110000,
		TotalDisbursed: 65000,
		Ahead:          2,
		OnTrack:        5,
		Behind:         2,
		Overdue:        1,
		DueSoon:        2,
		High:           1,
		Medium:         4,
		Low:            4,
		DueSoonWindow:  14,
	}
	report := buildTrendReportText(current, previous, time.Date(2025, 5, 1, 10, 0, 0, 0, time.UTC))
	if !strings.Contains(report, "Group Scholar Pacing Trend Report") {
		t.Fatalf("expected trend report title")
	}
	if !strings.Contains(report, "Delta: records +1") {
		t.Fatalf("expected record delta in report")
	}
	if !strings.Contains(report, "Pace mix:") {
		t.Fatalf("expected pace mix section")
	}
	if !strings.Contains(report, "Risk mix:") {
		t.Fatalf("expected risk mix section")
	}
}
