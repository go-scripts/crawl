package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CrawlResult represents a single crawled page result
type CrawlResult struct {
	URL        string
	Title      string
	StatusCode int
	Links      int
	Type       string
	Error      string
}

// ResultsTable manages the crawl results display
type ResultsTable struct {
	viewport    viewport.Model
	results     []CrawlResult
	width       int
	height      int
	headerStyle lipgloss.Style
	cellStyle   lipgloss.Style
	style       lipgloss.Style
}

// NewResultsTable creates a new results table
func NewResultsTable() *ResultsTable {
	t := &ResultsTable{
		results: make([]CrawlResult, 0),
		headerStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")),
		cellStyle: lipgloss.NewStyle().
			PaddingLeft(1).
			PaddingRight(1),
		style: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("35")),
	}
	t.viewport = viewport.New(0, 0)
	return t
}

// SetSize updates the table dimensions
func (t *ResultsTable) SetSize(width, height int) {
	t.width = width
	t.height = height
	t.viewport.Width = width - 4
	t.viewport.Height = height - 4
}

// Update handles UI updates
func (t *ResultsTable) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			t.viewport.LineUp(1)
		case "down", "j":
			t.viewport.LineDown(1)
		case "pgup":
			t.viewport.HalfViewUp()
		case "pgdown":
			t.viewport.HalfViewDown()
		}
	}

	// Handle viewport updates
	var viewportCmd tea.Cmd
	t.viewport, viewportCmd = t.viewport.Update(msg)
	cmd = viewportCmd

	return cmd
}

// View renders the table
func (t *ResultsTable) View() string {
	if len(t.results) == 0 {
		return t.style.Render(infoStyle.Render("No results yet"))
	}

	// Calculate column widths
	urlWidth := min(40, t.width/3)
	titleWidth := min(30, t.width/4)
	// statsWidth := min(20, t.width/6)

	// Build header
	header := t.headerStyle.Render(fmt.Sprintf(
		"%-*s %-*s %8s %8s %-12s",
		urlWidth, "URL",
		titleWidth, "Title",
		"Status",
		"Links",
		"Type",
	))

	// Build rows
	var rows []string
	for _, result := range t.results {
		status := fmt.Sprintf("%d", result.StatusCode)
		links := fmt.Sprintf("%d", result.Links)

		row := t.cellStyle.Render(fmt.Sprintf(
			"%-*s %-*s %8s %8s %-12s",
			urlWidth, truncate(result.URL, urlWidth),
			titleWidth, truncate(result.Title, titleWidth),
			status,
			links,
			result.Type,
		))

		if result.Error != "" {
			row = errorStyle.Render(row)
		} else if result.StatusCode >= 400 {
			row = warningStyle.Render(row)
		}

		rows = append(rows, row)
	}

	// Combine content
	content := header + "\n" + strings.Join(rows, "\n")

	// Update viewport content
	t.viewport.SetContent(content)

	// Add statistics
	stats := fmt.Sprintf(
		"\nTotal Pages: %d | Success: %d | Errors: %d",
		len(t.results),
		t.successCount(),
		t.errorCount(),
	)

	return t.style.Width(t.width).Render(
		t.viewport.View() + "\n" + infoStyle.Render(stats),
	)
}

// AddResult adds a new crawl result
func (t *ResultsTable) AddResult(result CrawlResult) {
	t.results = append(t.results, result)
	// Scroll to bottom if we're already at the bottom
	if t.viewport.AtBottom() {
		t.viewport.GotoBottom()
	}
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncate(s string, w int) string {
	if len(s) <= w {
		return s
	}
	return s[:w-3] + "..."
}

func (t *ResultsTable) successCount() int {
	count := 0
	for _, r := range t.results {
		if r.Error == "" && r.StatusCode < 400 {
			count++
		}
	}
	return count
}

func (t *ResultsTable) errorCount() int {
	count := 0
	for _, r := range t.results {
		if r.Error != "" || r.StatusCode >= 400 {
			count++
		}
	}
	return count
}
