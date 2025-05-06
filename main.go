package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
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
	Label    string
	Delta    float64
	Percent  float64
	Expected float64
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
	ready             bool
	width             int
	height            int
	updatedAt         time.Time
	checkinWindowDays int
	sortMode          string
	filterMode        string
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
	checkinWindow := flag.Int("checkin-window", 14, "days before a check-in is considered due soon")
	flag.Parse()

	records, err := loadData(*dataPath)
	if err != nil {
		fmt.Println("error loading data:", err)
		os.Exit(1)
	}

	now := time.Now()
	baseItems := buildItems(records, now, *checkinWindow)
	items := sortItems(applyFilter(baseItems, "all"), "priority")
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
		summary:           buildSummary(items, *checkinWindow),
		detail:            buildDetail(items, 0),
		updatedAt:         now,
		checkinWindowDays: *checkinWindow,
		sortMode:          "priority",
		filterMode:        "all",
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
		checkLabel := formatCheckinBadge(check)
		riskLabel := renderRiskLabel(risk)
		desc := fmt.Sprintf("%s · %s disbursed · %s · %s · %s", record.Cohort, percent, label, checkLabel, riskLabel)
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
	return paceStatus{
		Label:    paceLabel(percent - expected),
		Delta:    percent - expected,
		Percent:  percent,
		Expected: expected,
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

func buildSummary(items []awardItem, dueSoonDays int) string {
	if len(items) == 0 {
		return "No records loaded."
	}
	var totalAwarded, totalDisbursed float64
	ahead := 0
	onTrack := 0
	behind := 0
	overdue := 0
	dueSoon := 0
	high := 0
	medium := 0
	low := 0
	upcoming := make([]string, 0, len(items))
	for _, item := range items {
		record := item.data
		totalAwarded += record.Amount
		totalDisbursed += record.DisbursedToDate
		switch item.pace.Label {
		case "Ahead":
			ahead++
		case "Behind":
			behind++
		default:
			onTrack++
		}
		if item.check.Label == "Overdue" {
			overdue++
		}
		if item.check.Label == "Due Soon" {
			dueSoon++
		}
		switch item.risk.Level {
		case "High":
			high++
		case "Medium":
			medium++
		default:
			low++
		}
		if !item.check.Date.IsZero() && item.check.Label != "Overdue" {
			upcoming = append(upcoming, item.check.Date.Format("Jan 2")+" · "+record.Scholar)
		}
	}
	completion := totalDisbursed / totalAwarded
	sort.Strings(upcoming)
	preview := strings.Join(upcoming, " | ")
	if preview == "" {
		preview = "None"
	} else if len(preview) > 64 {
		preview = preview[:64] + "…"
	}
	return fmt.Sprintf("$%0.0f awarded · $%0.0f disbursed (%0.1f%%) · Pace %d ahead / %d on / %d behind · Risk %d high / %d med / %d low · %d overdue · %d due in %d days · Next: %s",
		totalAwarded,
		totalDisbursed,
		completion*100,
		ahead,
		onTrack,
		behind,
		high,
		medium,
		low,
		overdue,
		dueSoon,
		dueSoonDays,
		preview,
	)
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
	return fmt.Sprintf(
		"Scholar: %s\nCohort: %s\nOwner: %s\nStatus: %s\nAwarded: $%0.0f\nDisbursed: $%0.0f (%0.1f%%)\nExpected: %0.1f%%\nPace: %s (%0.1f%%)\nRisk: %s\nCheck-in: %s\nNotes: %s",
		record.Scholar,
		record.Cohort,
		record.Owner,
		record.Status,
		record.Amount,
		record.DisbursedToDate,
		pace.Percent*100,
		pace.Expected*100,
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
	m.summary = buildSummary(m.items, m.checkinWindowDays)
	return m, cmd
}

func (m model) View() string {
	if !m.ready {
		return "Loading award pacing console..."
	}

	header := headerStyle.Render("Group Scholar Award Pacing Console")
	meta := subtle.Render(fmt.Sprintf("Press / to filter · s to sort (%s) · f to focus (%s) · r to refresh timestamp · q to quit", m.sortMode, m.filterMode))
	stamp := subtle.Render("Updated " + m.updatedAt.Format("Jan 2 15:04"))

	left := panel.Render(m.list.View())
	right := panel.Render(m.detail)

	columns := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	return strings.Join([]string{
		header,
		meta,
		stamp,
		accent.Render(m.summary),
		columns,
	}, "\n\n")
}
