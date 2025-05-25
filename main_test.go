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
