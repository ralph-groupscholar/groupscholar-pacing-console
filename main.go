package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Disbursement struct {
	Scholar         string  `json:"scholar"`
	Cohort          string  `json:"cohort"`
	Amount          float64 `json:"amount"`
	DisbursedToDate float64 `json:"disbursed_to_date"`
	AwardDate       string  `json:"award_date"`
	TargetDate      string  `json:"target_date"`
	NextCheckin     string  `json:"next_checkin"`
	Owner           string  `json:"owner"`
	Status          string  `json:"status"`
	Notes           string  `json:"notes"`
}

type paceStatus struct {
	Label          string
	Delta          float64
	Percent        float64
	Expected       float64
	ExpectedAmount float64
	GapAmount      float64
}

type checkinStatus struct {
	Label string
	Days  int
	Date  time.Time
}

type awardItem struct {
	title string
	desc  string
	data  Disbursement
	pace  paceStatus
	check checkinStatus
	risk  riskStatus
}

func (a awardItem) Title() string       { return a.title }
func (a awardItem) Description() string { return a.desc }
func (a awardItem) FilterValue() string { return a.title }

type model struct {
	list              list.Model
	items             []awardItem
	baseItems         []awardItem
	records           []Disbursement
	summary           string
	detail            string
	insights          string
	ready             bool
	width             int
	height            int
	updatedAt         time.Time
	checkinWindowDays int
	sortMode          string
	filterMode        string
	showInsights      bool
}

type summaryMetrics struct {
	Count          int
	TotalAwarded   float64
	TotalDisbursed float64
	TotalExpected  float64
	TotalGap       float64
	Completion     float64
	Ahead          int
	OnTrack        int
	Behind         int
	Overdue        int
	DueSoon        int
	High           int
	Medium         int
	Low            int
	Upcoming       []string
}

type riskStatus struct {
	Level string
	Flags []string
	Score int
}

var (
	accent       = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	subtle       = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	panel        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	headerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	statusAhead  = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	statusOn     = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)
	statusBehind = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
)

func main() {
	dataPath := flag.String("data", "data/disbursements.json", "path to disbursement data")
	defaultDBURL := os.Getenv("PACECONSOLE_DATABASE_URL")
	if defaultDBURL == "" {
		defaultDBURL = os.Getenv("DATABASE_URL")
	}
	dbURL := flag.String("db-url", defaultDBURL, "Postgres connection string (optional)")
	checkinWindow := flag.Int("checkin-window", 14, "days before a check-in is considered due soon")
	source := flag.String("source", "file", "data source: file or db")
	dbSync := flag.Bool("db-sync", false, "write a pacing snapshot to Postgres (requires GS_PACING_DB_DSN or -db-url)")
	exportPath := flag.String("export", "", "export snapshot to csv or json (path)")
	exportFilter := flag.String("export-filter", "all", "export filter: all, risk, high")
	flag.Parse()

	var (
		records []Disbursement
		err     error
	)
	if strings.EqualFold(*source, "db") {
		records, err = loadDataFromDB(*dbURL)
	} else {
		records, err = loadData(*dataPath)
	}
	if err != nil {
		fmt.Println("error loading data:", err)
		os.Exit(1)
	}

	now := time.Now()
	baseItems := buildItems(records, now, *checkinWindow)
	if *dbSync {
		if err := syncToDatabase(baseItems, *checkinWindow, *dbURL); err != nil {
			fmt.Println("error syncing database:", err)
			os.Exit(1)
		}
		return
	}
	if strings.TrimSpace(*exportPath) != "" {
		filterMode, err := normalizeFilterMode(*exportFilter)
		if err != nil {
			fmt.Println("error exporting snapshot:", err)
			os.Exit(1)
		}
		items := sortItems(applyFilter(baseItems, filterMode), "priority")
		metrics := calculateSummaryMetrics(items)
		if err := exportSnapshot(*exportPath, items, metrics, now, *checkinWindow); err != nil {
			fmt.Println("error exporting snapshot:", err)
			os.Exit(1)
		}
		fmt.Printf("Exported %d awards to %s\n", len(items), *exportPath)
		return
	}
	items := sortItems(applyFilter(baseItems, "all"), "priority")
	metrics := calculateSummaryMetrics(items)
	listModel := list.New(itemsToList(items), list.NewDefaultDelegate(), 0, 0)
	listModel.Title = "Award Pacing Console"
	listModel.SetShowStatusBar(false)
	listModel.SetFilteringEnabled(true)
	listModel.SetShowHelp(false)

	m := model{
		list:              listModel,
		items:             items,
		baseItems:         baseItems,
		records:           records,
		summary:           buildSummary(metrics, *checkinWindow),
		detail:            buildDetail(items, 0),
		insights:          buildInsights(items),
		updatedAt:         now,
		checkinWindowDays: *checkinWindow,
		sortMode:          "priority",
		filterMode:        "all",
		showInsights:      false,
	}

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Println("error running program:", err)
		os.Exit(1)
	}
}

func loadData(path string) ([]Disbursement, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var records []Disbursement
	if err := json.Unmarshal(content, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func buildItems(records []Disbursement, now time.Time, windowDays int) []awardItem {
	items := make([]awardItem, 0, len(records))
	checkinWindow := windowDays
	if checkinWindow < 0 {
		checkinWindow = 0
	}
	for _, record := range records {
		pace := calculatePace(record, now)
		check := calculateCheckin(record, now, checkinWindow)
		risk := calculateRisk(pace, check)
		label := renderPaceLabel(pace)
		percent := fmt.Sprintf("%0.1f%%", pace.Percent*100)
		gapLabel := formatSignedCurrency(pace.GapAmount)
		checkLabel := formatCheckinBadge(check)
		riskLabel := renderRiskLabel(risk)
		desc := fmt.Sprintf("%s · %s disbursed · %s · %s · Gap %s · %s", record.Cohort, percent, label, checkLabel, gapLabel, riskLabel)
		items = append(items, awardItem{
			title: fmt.Sprintf("%s (%s)", record.Scholar, record.Owner),
			desc:  desc,
			data:  record,
			pace:  pace,
			check: check,
			risk:  risk,
		})
	}
	return items
}

func sortItems(items []awardItem, mode string) []awardItem {
	sorted := make([]awardItem, len(items))
	copy(sorted, items)
	switch mode {
	case "alpha":
		sort.SliceStable(sorted, func(i, j int) bool {
			return strings.ToLower(sorted[i].data.Scholar) < strings.ToLower(sorted[j].data.Scholar)
		})
	case "priority":
		sort.SliceStable(sorted, func(i, j int) bool {
			left := sorted[i]
			right := sorted[j]
			if checkinRank(left.check.Label) != checkinRank(right.check.Label) {
				return checkinRank(left.check.Label) < checkinRank(right.check.Label)
			}
			if paceRank(left.pace.Label) != paceRank(right.pace.Label) {
				return paceRank(left.pace.Label) < paceRank(right.pace.Label)
			}
			if !left.check.Date.Equal(right.check.Date) {
				if left.check.Date.IsZero() {
					return false
				}
				if right.check.Date.IsZero() {
					return true
				}
				return left.check.Date.Before(right.check.Date)
			}
			return strings.ToLower(left.data.Scholar) < strings.ToLower(right.data.Scholar)
		})
	}
	return sorted
}

func applyFilter(items []awardItem, mode string) []awardItem {
	filtered := make([]awardItem, 0, len(items))
	if mode == "all" {
		return items
	}
	for _, item := range items {
		if mode == "risk" {
			if item.pace.Label == "Behind" || item.check.Label == "Overdue" || item.check.Label == "Due Soon" {
				filtered = append(filtered, item)
			}
			continue
		}
		if mode == "high" && item.risk.Level == "High" {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func normalizeFilterMode(mode string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "" || normalized == "all" {
		return "all", nil
	}
	if normalized == "risk" {
		return "risk", nil
	}
	if normalized == "high" {
		return "high", nil
	}
	return "", fmt.Errorf("unknown filter mode: %s", mode)
}

func itemsToList(items []awardItem) []list.Item {
	listItems := make([]list.Item, 0, len(items))
	for _, item := range items {
		listItems = append(listItems, item)
	}
	return listItems
}

func calculatePace(record Disbursement, now time.Time) paceStatus {
	awardDate := parseDateOrNow(record.AwardDate, now)
	targetDate := parseDateOrNow(record.TargetDate, now)

	totalDays := math.Max(1, targetDate.Sub(awardDate).Hours()/24)
	elapsedDays := math.Max(0, now.Sub(awardDate).Hours()/24)
	expected := clamp(elapsedDays/totalDays, 0, 1)
	percent := clamp(record.DisbursedToDate/record.Amount, 0, 1)
	expectedAmount := record.Amount * expected
	gapAmount := record.DisbursedToDate - expectedAmount
	return paceStatus{
		Label:          paceLabel(percent - expected),
		Delta:          percent - expected,
		Percent:        percent,
		Expected:       expected,
		ExpectedAmount: expectedAmount,
		GapAmount:      gapAmount,
	}
}

func calculateCheckin(record Disbursement, now time.Time, windowDays int) checkinStatus {
	if record.NextCheckin == "" {
		return checkinStatus{Label: "Unscheduled"}
	}
	checkDate, ok := parseDateOptional(record.NextCheckin)
	if !ok {
		return checkinStatus{Label: "Unscheduled"}
	}
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	checkDate = time.Date(checkDate.Year(), checkDate.Month(), checkDate.Day(), 0, 0, 0, 0, checkDate.Location())
	daysUntil := int(math.Round(checkDate.Sub(nowDate).Hours() / 24))
	label := "Scheduled"
	if daysUntil < 0 {
		label = "Overdue"
	} else if daysUntil <= windowDays {
		label = "Due Soon"
	}
	return checkinStatus{Label: label, Days: daysUntil, Date: checkDate}
}

func paceLabel(delta float64) string {
	if delta >= 0.1 {
		return "Ahead"
	}
	if delta <= -0.1 {
		return "Behind"
	}
	return "On Track"
}

func checkinRank(label string) int {
	switch label {
	case "Overdue":
		return 0
	case "Due Soon":
		return 1
	case "Scheduled":
		return 2
	default:
		return 3
	}
}

func paceRank(label string) int {
	switch label {
	case "Behind":
		return 0
	case "On Track":
		return 1
	default:
		return 2
	}
}

func renderPaceLabel(p paceStatus) string {
	switch p.Label {
	case "Ahead":
		return statusAhead.Render("Ahead")
	case "Behind":
		return statusBehind.Render("Behind")
	default:
		return statusOn.Render("On Track")
	}
}

func renderCheckinLabel(c checkinStatus) string {
	switch c.Label {
	case "Overdue":
		return statusBehind.Render("Overdue")
	case "Due Soon":
		return statusOn.Render("Due Soon")
	case "Scheduled":
		return subtle.Render("Scheduled")
	default:
		return subtle.Render("Unscheduled")
	}
}

func renderRiskLabel(r riskStatus) string {
	switch r.Level {
	case "High":
		return statusBehind.Render("Risk: High")
	case "Medium":
		return statusOn.Render("Risk: Medium")
	default:
		return subtle.Render("Risk: Low")
	}
}

func formatDaysLabel(days int) string {
	if days == 0 {
		return "today"
	}
	if days < 0 {
		return fmt.Sprintf("%dd overdue", -days)
	}
	return fmt.Sprintf("in %dd", days)
}

func formatCheckinBadge(c checkinStatus) string {
	if c.Label == "Unscheduled" {
		return "Check-in: " + renderCheckinLabel(c)
	}
	return fmt.Sprintf("Check-in: %s (%s)", renderCheckinLabel(c), formatDaysLabel(c.Days))
}

func formatSignedCurrency(value float64) string {
	sign := "-"
	if value >= 0 {
		sign = "+"
	}
	return fmt.Sprintf("%s$%0.0f", sign, math.Abs(value))
}

func calculateRisk(pace paceStatus, check checkinStatus) riskStatus {
	score := 0
	flags := make([]string, 0, 3)
	if pace.Label == "Behind" {
		score += 2
		flags = append(flags, "Behind pace")
	}
	if check.Label == "Overdue" {
		score += 2
		flags = append(flags, "Check-in overdue")
	}
	if check.Label == "Due Soon" {
		score++
		flags = append(flags, "Check-in due soon")
	}
	if check.Label == "Unscheduled" {
		score++
		flags = append(flags, "Check-in unscheduled")
	}
	if pace.Label == "Ahead" {
		score--
	}
	level := "Low"
	if score >= 3 {
		level = "High"
	} else if score >= 2 {
		level = "Medium"
	}
	return riskStatus{Level: level, Flags: flags, Score: score}
}

func parseDateOrNow(value string, fallback time.Time) time.Time {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseDateOptional(value string) (time.Time, bool) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func calculateSummaryMetrics(items []awardItem) summaryMetrics {
	metrics := summaryMetrics{
		Count:    len(items),
		Upcoming: make([]string, 0, len(items)),
	}
	for _, item := range items {
		record := item.data
		metrics.TotalAwarded += record.Amount
		metrics.TotalDisbursed += record.DisbursedToDate
		metrics.TotalExpected += item.pace.ExpectedAmount
		metrics.TotalGap += item.pace.GapAmount
		switch item.pace.Label {
		case "Ahead":
			metrics.Ahead++
		case "Behind":
			metrics.Behind++
		default:
			metrics.OnTrack++
		}
		if item.check.Label == "Overdue" {
			metrics.Overdue++
		}
		if item.check.Label == "Due Soon" {
			metrics.DueSoon++
		}
		switch item.risk.Level {
		case "High":
			metrics.High++
		case "Medium":
			metrics.Medium++
		default:
			metrics.Low++
		}
		if !item.check.Date.IsZero() && item.check.Label != "Overdue" {
			metrics.Upcoming = append(metrics.Upcoming, item.check.Date.Format("Jan 2")+" · "+record.Scholar)
		}
	}
	if metrics.TotalAwarded > 0 {
		metrics.Completion = metrics.TotalDisbursed / metrics.TotalAwarded
	}
	return metrics
}

func buildSummary(metrics summaryMetrics, dueSoonDays int) string {
	if metrics.Count == 0 {
		return "No records loaded."
	}
	sort.Strings(metrics.Upcoming)
	preview := strings.Join(metrics.Upcoming, " | ")
	if preview == "" {
		preview = "None"
	} else if len(preview) > 64 {
		preview = preview[:64] + "…"
	}
	return fmt.Sprintf("$%0.0f awarded · $%0.0f disbursed (%0.1f%%) · Expected $%0.0f · Gap %s · Pace %d ahead / %d on / %d behind · Risk %d high / %d med / %d low · %d overdue · %d due in %d days · Next: %s",
		metrics.TotalAwarded,
		metrics.TotalDisbursed,
		metrics.Completion*100,
		metrics.TotalExpected,
		formatSignedCurrency(metrics.TotalGap),
		metrics.Ahead,
		metrics.OnTrack,
		metrics.Behind,
		metrics.High,
		metrics.Medium,
		metrics.Low,
		metrics.Overdue,
		metrics.DueSoon,
		dueSoonDays,
		preview,
	)
}

type exportSummary struct {
	Count          int      `json:"count"`
	TotalAwarded   float64  `json:"total_awarded"`
	TotalDisbursed float64  `json:"total_disbursed"`
	TotalExpected  float64  `json:"total_expected"`
	TotalGap       float64  `json:"total_gap"`
	Completion     float64  `json:"completion"`
	Ahead          int      `json:"ahead"`
	OnTrack        int      `json:"on_track"`
	Behind         int      `json:"behind"`
	Overdue        int      `json:"overdue"`
	DueSoon        int      `json:"due_soon"`
	High           int      `json:"high"`
	Medium         int      `json:"medium"`
	Low            int      `json:"low"`
	Upcoming       []string `json:"upcoming"`
}

type exportItem struct {
	Scholar         string   `json:"scholar"`
	Cohort          string   `json:"cohort"`
	Owner           string   `json:"owner"`
	Status          string   `json:"status"`
	Amount          float64  `json:"amount"`
	DisbursedToDate float64  `json:"disbursed_to_date"`
	AwardDate       string   `json:"award_date"`
	TargetDate      string   `json:"target_date"`
	NextCheckin     string   `json:"next_checkin"`
	PaceLabel       string   `json:"pace_label"`
	PacePercent     float64  `json:"pace_percent"`
	PaceDelta       float64  `json:"pace_delta"`
	ExpectedPercent float64  `json:"expected_percent"`
	ExpectedAmount  float64  `json:"expected_amount"`
	GapAmount       float64  `json:"gap_amount"`
	CheckinLabel    string   `json:"checkin_label"`
	CheckinDays     *int     `json:"checkin_days,omitempty"`
	RiskLevel       string   `json:"risk_level"`
	RiskScore       int      `json:"risk_score"`
	RiskFlags       []string `json:"risk_flags,omitempty"`
	Notes           string   `json:"notes"`
}

type exportSnapshotPayload struct {
	GeneratedAt       string        `json:"generated_at"`
	CheckinWindowDays int           `json:"checkin_window_days"`
	Summary           exportSummary `json:"summary"`
	Items             []exportItem  `json:"items"`
}

type ownerSummary struct {
	Owner    string
	Awards   int
	High     int
	Overdue  int
	DueSoon  int
	GapTotal float64
}

type cohortSummary struct {
	Cohort     string
	Awards     int
	Behind     int
	GapTotal   float64
	Completion float64
}

type statusSummary struct {
	Status string
	Count  int
}

func exportSnapshot(path string, items []awardItem, metrics summaryMetrics, generatedAt time.Time, checkinWindow int) error {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		ext = ".csv"
		path = path + ext
	}
	if ext == ".json" {
		return exportSnapshotJSON(path, items, metrics, generatedAt, checkinWindow)
	}
	if ext == ".csv" {
		return exportSnapshotCSV(path, items, metrics, generatedAt, checkinWindow)
	}
	return fmt.Errorf("unsupported export format: %s", ext)
}

func exportSnapshotJSON(path string, items []awardItem, metrics summaryMetrics, generatedAt time.Time, checkinWindow int) error {
	payload := exportSnapshotPayload{
		GeneratedAt:       generatedAt.Format(time.RFC3339),
		CheckinWindowDays: checkinWindow,
		Summary: exportSummary{
			Count:          metrics.Count,
			TotalAwarded:   metrics.TotalAwarded,
			TotalDisbursed: metrics.TotalDisbursed,
			TotalExpected:  metrics.TotalExpected,
			TotalGap:       metrics.TotalGap,
			Completion:     metrics.Completion,
			Ahead:          metrics.Ahead,
			OnTrack:        metrics.OnTrack,
			Behind:         metrics.Behind,
			Overdue:        metrics.Overdue,
			DueSoon:        metrics.DueSoon,
			High:           metrics.High,
			Medium:         metrics.Medium,
			Low:            metrics.Low,
			Upcoming:       metrics.Upcoming,
		},
		Items: make([]exportItem, 0, len(items)),
	}
	for _, item := range items {
		record := item.data
		checkinDays := (*int)(nil)
		if item.check.Label != "Unscheduled" {
			days := item.check.Days
			checkinDays = &days
		}
		payload.Items = append(payload.Items, exportItem{
			Scholar:         record.Scholar,
			Cohort:          record.Cohort,
			Owner:           record.Owner,
			Status:          record.Status,
			Amount:          record.Amount,
			DisbursedToDate: record.DisbursedToDate,
			AwardDate:       record.AwardDate,
			TargetDate:      record.TargetDate,
			NextCheckin:     record.NextCheckin,
			PaceLabel:       item.pace.Label,
			PacePercent:     item.pace.Percent,
			PaceDelta:       item.pace.Delta,
			ExpectedPercent: item.pace.Expected,
			ExpectedAmount:  item.pace.ExpectedAmount,
			GapAmount:       item.pace.GapAmount,
			CheckinLabel:    item.check.Label,
			CheckinDays:     checkinDays,
			RiskLevel:       item.risk.Level,
			RiskScore:       item.risk.Score,
			RiskFlags:       item.risk.Flags,
			Notes:           record.Notes,
		})
	}
	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func exportSnapshotCSV(path string, items []awardItem, metrics summaryMetrics, generatedAt time.Time, checkinWindow int) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{
		"generated_at",
		"checkin_window_days",
		"summary_count",
		"summary_total_awarded",
		"summary_total_disbursed",
		"summary_total_expected",
		"summary_total_gap",
		"summary_completion",
		"summary_ahead",
		"summary_on_track",
		"summary_behind",
		"summary_overdue",
		"summary_due_soon",
		"summary_high",
		"summary_medium",
		"summary_low",
	}); err != nil {
		return err
	}
	if err := writer.Write([]string{
		generatedAt.Format(time.RFC3339),
		fmt.Sprintf("%d", checkinWindow),
		fmt.Sprintf("%d", metrics.Count),
		fmt.Sprintf("%0.2f", metrics.TotalAwarded),
		fmt.Sprintf("%0.2f", metrics.TotalDisbursed),
		fmt.Sprintf("%0.2f", metrics.TotalExpected),
		fmt.Sprintf("%0.2f", metrics.TotalGap),
		fmt.Sprintf("%0.4f", metrics.Completion),
		fmt.Sprintf("%d", metrics.Ahead),
		fmt.Sprintf("%d", metrics.OnTrack),
		fmt.Sprintf("%d", metrics.Behind),
		fmt.Sprintf("%d", metrics.Overdue),
		fmt.Sprintf("%d", metrics.DueSoon),
		fmt.Sprintf("%d", metrics.High),
		fmt.Sprintf("%d", metrics.Medium),
		fmt.Sprintf("%d", metrics.Low),
	}); err != nil {
		return err
	}

	if err := writer.Write([]string{
		"scholar",
		"cohort",
		"owner",
		"status",
		"amount",
		"disbursed_to_date",
		"award_date",
		"target_date",
		"next_checkin",
		"pace_label",
		"pace_percent",
		"pace_delta",
		"expected_percent",
		"expected_amount",
		"gap_amount",
		"checkin_label",
		"checkin_days",
		"risk_level",
		"risk_score",
		"risk_flags",
		"notes",
	}); err != nil {
		return err
	}

	for _, item := range items {
		record := item.data
		checkinDays := ""
		if item.check.Label != "Unscheduled" {
			checkinDays = fmt.Sprintf("%d", item.check.Days)
		}
		if err := writer.Write([]string{
			record.Scholar,
			record.Cohort,
			record.Owner,
			record.Status,
			fmt.Sprintf("%0.2f", record.Amount),
			fmt.Sprintf("%0.2f", record.DisbursedToDate),
			record.AwardDate,
			record.TargetDate,
			record.NextCheckin,
			item.pace.Label,
			fmt.Sprintf("%0.4f", item.pace.Percent),
			fmt.Sprintf("%0.4f", item.pace.Delta),
			fmt.Sprintf("%0.4f", item.pace.Expected),
			fmt.Sprintf("%0.2f", item.pace.ExpectedAmount),
			fmt.Sprintf("%0.2f", item.pace.GapAmount),
			item.check.Label,
			checkinDays,
			item.risk.Level,
			fmt.Sprintf("%d", item.risk.Score),
			strings.Join(item.risk.Flags, "; "),
			record.Notes,
		}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func buildOwnerSummaries(items []awardItem) []ownerSummary {
	index := make(map[string]*ownerSummary)
	for _, item := range items {
		owner := item.data.Owner
		entry, ok := index[owner]
		if !ok {
			entry = &ownerSummary{Owner: owner}
			index[owner] = entry
		}
		entry.Awards++
		entry.GapTotal += item.pace.GapAmount
		if item.risk.Level == "High" {
			entry.High++
		}
		if item.check.Label == "Overdue" {
			entry.Overdue++
		}
		if item.check.Label == "Due Soon" {
			entry.DueSoon++
		}
	}
	summaries := make([]ownerSummary, 0, len(index))
	for _, entry := range index {
		summaries = append(summaries, *entry)
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].High != summaries[j].High {
			return summaries[i].High > summaries[j].High
		}
		if summaries[i].Overdue != summaries[j].Overdue {
			return summaries[i].Overdue > summaries[j].Overdue
		}
		if summaries[i].GapTotal != summaries[j].GapTotal {
			return summaries[i].GapTotal < summaries[j].GapTotal
		}
		return strings.ToLower(summaries[i].Owner) < strings.ToLower(summaries[j].Owner)
	})
	return summaries
}

func buildCohortSummaries(items []awardItem) []cohortSummary {
	index := make(map[string]*cohortSummary)
	for _, item := range items {
		cohort := item.data.Cohort
		entry, ok := index[cohort]
		if !ok {
			entry = &cohortSummary{Cohort: cohort}
			index[cohort] = entry
		}
		entry.Awards++
		entry.GapTotal += item.pace.GapAmount
		if item.pace.Label == "Behind" {
			entry.Behind++
		}
		if item.data.Amount > 0 {
			entry.Completion += item.data.DisbursedToDate / item.data.Amount
		}
	}
	summaries := make([]cohortSummary, 0, len(index))
	for _, entry := range index {
		if entry.Awards > 0 {
			entry.Completion = entry.Completion / float64(entry.Awards)
		}
		summaries = append(summaries, *entry)
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].Behind != summaries[j].Behind {
			return summaries[i].Behind > summaries[j].Behind
		}
		if summaries[i].GapTotal != summaries[j].GapTotal {
			return summaries[i].GapTotal < summaries[j].GapTotal
		}
		return strings.ToLower(summaries[i].Cohort) < strings.ToLower(summaries[j].Cohort)
	})
	return summaries
}

func buildStatusSummary(items []awardItem) []statusSummary {
	counts := make(map[string]int)
	for _, item := range items {
		status := strings.TrimSpace(item.data.Status)
		if status == "" {
			status = "Unspecified"
		}
		counts[status]++
	}
	summaries := make([]statusSummary, 0, len(counts))
	for status, count := range counts {
		summaries = append(summaries, statusSummary{Status: status, Count: count})
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].Count != summaries[j].Count {
			return summaries[i].Count > summaries[j].Count
		}
		return strings.ToLower(summaries[i].Status) < strings.ToLower(summaries[j].Status)
	})
	return summaries
}

func buildInsights(items []awardItem) string {
	if len(items) == 0 {
		return "No records loaded."
	}
	ownerSummaries := buildOwnerSummaries(items)
	cohortSummaries := buildCohortSummaries(items)
	statusSummaries := buildStatusSummary(items)

	ownerLines := make([]string, 0, 6)
	ownerLines = append(ownerLines, "Owner pulse (top risk):")
	for i, summary := range ownerSummaries {
		if i >= 5 {
			break
		}
		ownerLines = append(ownerLines, fmt.Sprintf("- %s · %d awards · %d high · %d overdue · %s gap",
			summary.Owner,
			summary.Awards,
			summary.High,
			summary.Overdue,
			formatSignedCurrency(summary.GapTotal),
		))
	}

	cohortLines := make([]string, 0, 6)
	cohortLines = append(cohortLines, "Cohort watchlist:")
	cohortCount := 0
	for _, summary := range cohortSummaries {
		if summary.Behind == 0 && summary.GapTotal >= 0 {
			continue
		}
		cohortLines = append(cohortLines, fmt.Sprintf("- %s · %d behind · %s gap · %0.1f%% complete",
			summary.Cohort,
			summary.Behind,
			formatSignedCurrency(summary.GapTotal),
			summary.Completion*100,
		))
		cohortCount++
		if cohortCount >= 4 {
			break
		}
	}
	if cohortCount == 0 {
		cohortLines = append(cohortLines, "- None")
	}

	statusLine := "Status mix: "
	statusParts := make([]string, 0, len(statusSummaries))
	for _, summary := range statusSummaries {
		statusParts = append(statusParts, fmt.Sprintf("%s %d", summary.Status, summary.Count))
	}
	statusLine += strings.Join(statusParts, " · ")

	return strings.Join([]string{
		strings.Join(ownerLines, "\n"),
		strings.Join(cohortLines, "\n"),
		statusLine,
	}, "\n\n")
}

func buildDetail(items []awardItem, index int) string {
	if len(items) == 0 || index < 0 || index >= len(items) {
		return "Select an award to see details."
	}
	item := items[index]
	record := item.data
	pace := item.pace
	check := item.check
	risk := item.risk
	checkinLine := "Not scheduled"
	if !check.Date.IsZero() {
		checkinLine = fmt.Sprintf("%s (%s)", check.Date.Format("Jan 2, 2006"), check.Label)
		if check.Days >= 0 {
			checkinLine = fmt.Sprintf("%s (in %d days, %s)", check.Date.Format("Jan 2, 2006"), check.Days, check.Label)
		} else {
			checkinLine = fmt.Sprintf("%s (%d days overdue)", check.Date.Format("Jan 2, 2006"), int(math.Abs(float64(check.Days))))
		}
	}
	riskLine := risk.Level
	if len(risk.Flags) > 0 {
		riskLine = fmt.Sprintf("%s (%s)", risk.Level, strings.Join(risk.Flags, "; "))
	}
	gapDirection := "behind"
	if pace.GapAmount >= 0 {
		gapDirection = "ahead"
	}
	return fmt.Sprintf(
		"Scholar: %s\nCohort: %s\nOwner: %s\nStatus: %s\nAwarded: $%0.0f\nDisbursed: $%0.0f (%0.1f%%)\nExpected: %0.1f%% ($%0.0f)\nGap vs expected: %s (%s)\nPace: %s (%0.1f%%)\nRisk: %s\nCheck-in: %s\nNotes: %s",
		record.Scholar,
		record.Cohort,
		record.Owner,
		record.Status,
		record.Amount,
		record.DisbursedToDate,
		pace.Percent*100,
		pace.Expected*100,
		pace.ExpectedAmount,
		formatSignedCurrency(pace.GapAmount),
		gapDirection,
		pace.Label,
		pace.Delta*100,
		riskLine,
		checkinLine,
		record.Notes,
	)
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listHeight := msg.Height - 12
		if listHeight < 8 {
			listHeight = 8
		}
		m.list.SetSize(msg.Width-4, listHeight)
		m.ready = true
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.updatedAt = time.Now()
			m.baseItems = buildItems(m.records, m.updatedAt, m.checkinWindowDays)
			m.items = sortItems(applyFilter(m.baseItems, m.filterMode), m.sortMode)
			m.list.SetItems(itemsToList(m.items))
			m.list.Select(0)
		case "i":
			m.showInsights = !m.showInsights
		case "s":
			if m.sortMode == "priority" {
				m.sortMode = "alpha"
			} else {
				m.sortMode = "priority"
			}
			m.items = sortItems(applyFilter(m.baseItems, m.filterMode), m.sortMode)
			m.list.SetItems(itemsToList(m.items))
			m.list.Select(0)
		case "f":
			switch m.filterMode {
			case "all":
				m.filterMode = "risk"
			case "risk":
				m.filterMode = "high"
			default:
				m.filterMode = "all"
			}
			m.items = sortItems(applyFilter(m.baseItems, m.filterMode), m.sortMode)
			m.list.SetItems(itemsToList(m.items))
			m.list.Select(0)
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	index := m.list.Index()
	m.detail = buildDetail(m.items, index)
	m.summary = buildSummary(calculateSummaryMetrics(m.items), m.checkinWindowDays)
	m.insights = buildInsights(m.items)
	return m, cmd
}

func (m model) View() string {
	if !m.ready {
		return "Loading award pacing console..."
	}

	header := headerStyle.Render("Group Scholar Award Pacing Console")
	meta := subtle.Render(fmt.Sprintf("Press / to filter · s to sort (%s) · f to focus (%s) · i for insights · r to refresh timestamp · q to quit", m.sortMode, m.filterMode))
	stamp := subtle.Render("Updated " + m.updatedAt.Format("Jan 2 15:04"))

	left := panel.Render(m.list.View())
	rightPanel := m.detail
	if m.showInsights {
		rightPanel = m.insights
	}
	right := panel.Render(rightPanel)

	columns := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	return strings.Join([]string{
		header,
		meta,
		stamp,
		accent.Render(m.summary),
		columns,
	}, "\n\n")
}
