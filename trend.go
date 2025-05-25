package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type trendSnapshot struct {
	GeneratedAt    string  `json:"generated_at"`
	RecordCount    int     `json:"record_count"`
	TotalAwarded   float64 `json:"total_awarded"`
	TotalDisbursed float64 `json:"total_disbursed"`
	Ahead          int     `json:"ahead"`
	OnTrack        int     `json:"on_track"`
	Behind         int     `json:"behind"`
	Overdue        int     `json:"overdue"`
	DueSoon        int     `json:"due_soon"`
	High           int     `json:"high"`
	Medium         int     `json:"medium"`
	Low            int     `json:"low"`
	DueSoonWindow  int     `json:"due_soon_window"`
}

type trendDelta struct {
	RecordCount    int     `json:"record_count"`
	TotalAwarded   float64 `json:"total_awarded"`
	TotalDisbursed float64 `json:"total_disbursed"`
	Ahead          int     `json:"ahead"`
	OnTrack        int     `json:"on_track"`
	Behind         int     `json:"behind"`
	Overdue        int     `json:"overdue"`
	DueSoon        int     `json:"due_soon"`
	High           int     `json:"high"`
	Medium         int     `json:"medium"`
	Low            int     `json:"low"`
}

type trendReportPayload struct {
	GeneratedAt string        `json:"generated_at"`
	Current     trendSnapshot `json:"current"`
	Previous    trendSnapshot `json:"previous"`
	Delta       trendDelta    `json:"delta"`
}

func writeTrendReport(path, format string, current, previous snapshotStats, generatedAt time.Time) error {
	format, err := normalizeReportFormat(path, format)
	if err != nil {
		return err
	}
	if format == "json" {
		payload := buildTrendReportPayload(current, previous, generatedAt)
		content, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		return writeReportOutput(path, content)
	}
	content := []byte(buildTrendReportText(current, previous, generatedAt))
	return writeReportOutput(path, content)
}

func buildTrendReportPayload(current, previous snapshotStats, generatedAt time.Time) trendReportPayload {
	return trendReportPayload{
		GeneratedAt: generatedAt.Format(time.RFC3339),
		Current:     buildTrendSnapshot(current),
		Previous:    buildTrendSnapshot(previous),
		Delta:       buildTrendDelta(current, previous),
	}
}

func buildTrendReportText(current, previous snapshotStats, generatedAt time.Time) string {
	currentSnapshot := buildTrendSnapshot(current)
	previousSnapshot := buildTrendSnapshot(previous)
	delta := buildTrendDelta(current, previous)

	windowNote := ""
	if currentSnapshot.DueSoonWindow != previousSnapshot.DueSoonWindow {
		windowNote = fmt.Sprintf(" (due-soon window changed from %d to %d days)", previousSnapshot.DueSoonWindow, currentSnapshot.DueSoonWindow)
	}

	lines := []string{
		"Group Scholar Pacing Trend Report",
		fmt.Sprintf("Generated: %s", generatedAt.Format(time.RFC3339)),
		"",
		fmt.Sprintf("Current snapshot: %s · %d records · $%0.2f awarded · $%0.2f disbursed%s",
			currentSnapshot.GeneratedAt,
			currentSnapshot.RecordCount,
			currentSnapshot.TotalAwarded,
			currentSnapshot.TotalDisbursed,
			windowNote,
		),
		fmt.Sprintf("Previous snapshot: %s · %d records · $%0.2f awarded · $%0.2f disbursed",
			previousSnapshot.GeneratedAt,
			previousSnapshot.RecordCount,
			previousSnapshot.TotalAwarded,
			previousSnapshot.TotalDisbursed,
		),
		"",
		fmt.Sprintf("Delta: records %s · awarded %s · disbursed %s",
			formatSignedInt(delta.RecordCount),
			formatSignedFloat(delta.TotalAwarded),
			formatSignedFloat(delta.TotalDisbursed),
		),
		fmt.Sprintf("Pace mix: Ahead %s · On track %s · Behind %s",
			formatSignedInt(delta.Ahead),
			formatSignedInt(delta.OnTrack),
			formatSignedInt(delta.Behind),
		),
		fmt.Sprintf("Check-ins: Overdue %s · Due soon %s",
			formatSignedInt(delta.Overdue),
			formatSignedInt(delta.DueSoon),
		),
		fmt.Sprintf("Risk mix: High %s · Medium %s · Low %s",
			formatSignedInt(delta.High),
			formatSignedInt(delta.Medium),
			formatSignedInt(delta.Low),
		),
	}

	return strings.Join(lines, "\n") + "\n"
}

func buildTrendSnapshot(stats snapshotStats) trendSnapshot {
	return trendSnapshot{
		GeneratedAt:    stats.GeneratedAt.Format(time.RFC3339),
		RecordCount:    stats.RecordCount,
		TotalAwarded:   stats.TotalAwarded,
		TotalDisbursed: stats.TotalDisbursed,
		Ahead:          stats.Ahead,
		OnTrack:        stats.OnTrack,
		Behind:         stats.Behind,
		Overdue:        stats.Overdue,
		DueSoon:        stats.DueSoon,
		High:           stats.High,
		Medium:         stats.Medium,
		Low:            stats.Low,
		DueSoonWindow:  stats.DueSoonWindow,
	}
}

func buildTrendDelta(current, previous snapshotStats) trendDelta {
	return trendDelta{
		RecordCount:    current.RecordCount - previous.RecordCount,
		TotalAwarded:   current.TotalAwarded - previous.TotalAwarded,
		TotalDisbursed: current.TotalDisbursed - previous.TotalDisbursed,
		Ahead:          current.Ahead - previous.Ahead,
		OnTrack:        current.OnTrack - previous.OnTrack,
		Behind:         current.Behind - previous.Behind,
		Overdue:        current.Overdue - previous.Overdue,
		DueSoon:        current.DueSoon - previous.DueSoon,
		High:           current.High - previous.High,
		Medium:         current.Medium - previous.Medium,
		Low:            current.Low - previous.Low,
	}
}

func formatSignedInt(value int) string {
	if value >= 0 {
		return fmt.Sprintf("+%d", value)
	}
	return fmt.Sprintf("%d", value)
}

func formatSignedFloat(value float64) string {
	if value >= 0 {
		return fmt.Sprintf("+$%0.2f", value)
	}
	return fmt.Sprintf("-$%0.2f", -value)
}
