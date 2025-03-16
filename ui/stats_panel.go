package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CrawlStats holds crawling statistics
type CrawlStats struct {
	TotalURLs         int
	ProcessedURLs     int
	QueuedURLs        int
	SuccessfulURLs    int
	FailedURLs        int
	ActiveWorkers     int
	StartTime         time.Time
	AverageTime       time.Duration
	LastProcessedURLs []string
}

// StatsPanel displays crawling statistics
type StatsPanel struct {
	stats      CrawlStats
	width      int
	height     int
	style      lipgloss.Style
	labelStyle lipgloss.Style
	valueStyle lipgloss.Style
}

func NewStatsPanel() *StatsPanel {
	return &StatsPanel{
		stats: CrawlStats{
			LastProcessedURLs: make([]string, 0, 5),
		},
		style: borderStyle.Copy().
			BorderForeground(lipgloss.Color("99")),
		labelStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Bold(true),
		valueStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")),
	}
}

func (s *StatsPanel) SetSize(width, height int) {
	s.width = width
	s.height = height
}

func (s *StatsPanel) Update(msg tea.Msg) tea.Cmd {
	return nil
}

func (s *StatsPanel) View() string {
	// Calculate progress percentage
	progress := 0.0
	if s.stats.TotalURLs > 0 {
		progress = float64(s.stats.ProcessedURLs) / float64(s.stats.TotalURLs) * 100
	}

	// Calculate pages per second
	pagesPerSecond := 0.0
	if !s.stats.StartTime.IsZero() {
		elapsed := time.Since(s.stats.StartTime).Seconds()
		if elapsed > 0 {
			pagesPerSecond = float64(s.stats.ProcessedURLs) / elapsed
		}
	}

	// Format the statistics
	stats := []struct {
		label string
		value string
	}{
		{"Progress", fmt.Sprintf("%.1f%% (%d/%d)", progress, s.stats.ProcessedURLs, s.stats.TotalURLs)},
		{"Queue Size", fmt.Sprintf("%d URLs", s.stats.QueuedURLs)},
		{"Success Rate", fmt.Sprintf("%.1f%% (%d/%d)",
			float64(s.stats.SuccessfulURLs)/float64(s.stats.ProcessedURLs)*100,
			s.stats.SuccessfulURLs, s.stats.ProcessedURLs)},
		{"Active Workers", fmt.Sprintf("%d", s.stats.ActiveWorkers)},
		{"Pages/Second", fmt.Sprintf("%.2f", pagesPerSecond)},
		{"Avg Response", fmt.Sprintf("%v", s.stats.AverageTime.Round(time.Millisecond))},
		{"Elapsed Time", s.formatElapsedTime()},
	}

	// Build the view
	var content strings.Builder
	content.WriteString(titleStyle.Render("Crawling Statistics") + "\n\n")

	// Format statistics in columns
	columnWidth := (s.width - 8) / 2 // Account for borders and padding
	for _, stat := range stats {
		line := fmt.Sprintf("%-*s %s\n",
			columnWidth,
			s.labelStyle.Render(stat.label+":"),
			s.valueStyle.Render(stat.value),
		)
		content.WriteString(line)
	}

	// Add recently processed URLs
	if len(s.stats.LastProcessedURLs) > 0 {
		content.WriteString("\nRecent URLs:\n")
		for _, url := range s.stats.LastProcessedURLs {
			content.WriteString(infoStyle.Render("• " + url + "\n"))
		}
	}

	return s.style.Width(s.width).Height(s.height).Render(content.String())
}

// UpdateStats updates the statistics
func (s *StatsPanel) UpdateStats(stats CrawlStats) {
	s.stats = stats
}

// Helper methods
func (s *StatsPanel) formatElapsedTime() string {
	if s.stats.StartTime.IsZero() {
		return "00:00:00"
	}
	elapsed := time.Since(s.stats.StartTime)
	return fmt.Sprintf("%02d:%02d:%02d",
		int(elapsed.Hours()),
		int(elapsed.Minutes())%60,
		int(elapsed.Seconds())%60,
	)
}

func (s *StatsPanel) AddProcessedURL(url string) {
	s.stats.LastProcessedURLs = append(s.stats.LastProcessedURLs, url)
	if len(s.stats.LastProcessedURLs) > 5 {
		s.stats.LastProcessedURLs = s.stats.LastProcessedURLs[1:]
	}
}
