package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LogLevel represents the severity of a log entry
type LogLevel int

const (
	LevelInfo LogLevel = iota
	LevelWarning
	LevelError
)

// LogEntry represents a single log message
type LogEntry struct {
	timestamp time.Time
	level     LogLevel
	message   string
}

// ErrorConsole manages error and warning messages
type ErrorConsole struct {
	viewport  viewport.Model
	entries   []LogEntry
	width     int
	height    int
	style     lipgloss.Style
	showLevel LogLevel // Filter to show only messages >= this level
}

// Styles for different log levels
var (
	errorLogStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	warningLogStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	infoLogStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("110"))

	timestampStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242")).
			Italic(true)
)

// NewErrorConsole creates a new error console
func NewErrorConsole() *ErrorConsole {
	e := &ErrorConsole{
		entries:   make([]LogEntry, 0),
		style:     borderStyle.Copy().BorderForeground(lipgloss.Color("196")),
		showLevel: LevelInfo,
	}
	e.viewport = viewport.New(0, 0)
	return e
}

// SetSize updates the console dimensions
func (e *ErrorConsole) SetSize(width, height int) {
	e.width = width
	e.height = height
	e.viewport.Width = width - 4
	e.viewport.Height = height - 4
}

// AddEntry adds a new log entry
func (e *ErrorConsole) AddEntry(level LogLevel, msg string) {
	entry := LogEntry{
		timestamp: time.Now(),
		level:     level,
		message:   msg,
	}
	e.entries = append(e.entries, entry)
	e.updateContent()
}

// Update handles UI updates
func (e *ErrorConsole) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			e.viewport.LineUp(1)
		case "down", "j":
			e.viewport.LineDown(1)
		case "pgup":
			e.viewport.HalfViewUp()
		case "pgdown":
			e.viewport.HalfViewDown()
		case "1":
			e.showLevel = LevelInfo
			e.updateContent()
		case "2":
			e.showLevel = LevelWarning
			e.updateContent()
		case "3":
			e.showLevel = LevelError
			e.updateContent()
		}
	}

	// Handle viewport updates
	var viewportCmd tea.Cmd
	e.viewport, viewportCmd = e.viewport.Update(msg)
	cmd = viewportCmd

	return cmd
}

// View renders the console
func (e *ErrorConsole) View() string {
	filterInfo := fmt.Sprintf(
		"\nFilter: %s (1:Info 2:Warn 3:Error)",
		e.levelString(e.showLevel),
	)

	stats := fmt.Sprintf(
		"Total: %d | Errors: %d | Warnings: %d",
		len(e.entries),
		e.countByLevel(LevelError),
		e.countByLevel(LevelWarning),
	)

	return e.style.Width(e.width).Render(
		e.viewport.View() +
			infoStyle.Render(filterInfo) + "\n" +
			infoStyle.Render(stats),
	)
}

// updateContent updates the viewport content
func (e *ErrorConsole) updateContent() {
	var sb strings.Builder

	for _, entry := range e.entries {
		if entry.level >= e.showLevel {
			timestamp := timestampStyle.Render(
				entry.timestamp.Format("15:04:05"),
			)

			var logStyle lipgloss.Style
			switch entry.level {
			case LevelError:
				logStyle = errorLogStyle
			case LevelWarning:
				logStyle = warningLogStyle
			default:
				logStyle = infoLogStyle
			}

			sb.WriteString(fmt.Sprintf(
				"%s [%s] %s\n",
				timestamp,
				logStyle.Render(e.levelString(entry.level)),
				entry.message,
			))
		}
	}

	e.viewport.SetContent(sb.String())
	// Scroll to bottom if we're already at the bottom
	if e.viewport.AtBottom() {
		e.viewport.GotoBottom()
	}
}

// Helper functions
func (e *ErrorConsole) levelString(level LogLevel) string {
	switch level {
	case LevelError:
		return "ERROR"
	case LevelWarning:
		return "WARN"
	default:
		return "INFO"
	}
}

func (e *ErrorConsole) countByLevel(level LogLevel) int {
	count := 0
	for _, entry := range e.entries {
		if entry.level == level {
			count++
		}
	}
	return count
}
