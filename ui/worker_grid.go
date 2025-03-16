package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// WorkerSpinner represents a single worker's spinner
type WorkerSpinner struct {
	spinner spinner.Model
	active  bool
	url     string
}

// WorkerGrid enhances the WorkerPanel with spinner visualization
type WorkerGrid struct {
	workers    []WorkerSpinner
	maxWorkers int
	columns    int
	style      lipgloss.Style
	width      int
	height     int
}

// Initialize a new worker grid
func NewWorkerGrid(maxWorkers int) *WorkerGrid {
	grid := &WorkerGrid{
		maxWorkers: maxWorkers,
		columns:    4, // Default to 4 columns
		style:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()),
		workers:    make([]WorkerSpinner, maxWorkers),
	}

	// Initialize spinners
	for i := range grid.workers {
		s := spinner.New()
		s.Spinner = spinner.Dot
		s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
		grid.workers[i] = WorkerSpinner{
			spinner: s,
			active:  false,
		}
	}

	return grid
}

// ActivateWorker activates a worker spinner and assigns it a URL
func (g *WorkerGrid) ActivateWorker(id int, url string) {
	if id < 0 || id >= g.maxWorkers {
		return
	}
	g.workers[id].active = true
	g.workers[id].url = url
	g.workers[id].spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
}

// DeactivateWorker deactivates a worker spinner
func (g *WorkerGrid) DeactivateWorker(id int) {
	if id < 0 || id >= g.maxWorkers {
		return
	}
	g.workers[id].active = false
	g.workers[id].url = ""
	g.workers[id].spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
}

// Update handles the spinner animations
func (g *WorkerGrid) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		g.width = msg.Width
		g.height = msg.Height
	}

	// Update active spinners
	for i := range g.workers {
		if g.workers[i].active {
			cmd := g.workers[i].spinner.Tick()
			cmds = append(cmds, cmd)
		}
	}

	return tea.Batch(cmds...)
}

// View renders the worker grid
func (g *WorkerGrid) View() string {
	// Calculate dimensions
	workerWidth := 10 // Width for each worker cell
	rows := (g.maxWorkers + g.columns - 1) / g.columns

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Workers (%d/%d active)\n\n", g.ActiveCount(), g.maxWorkers))

	// Render grid
	for row := 0; row < rows; row++ {
		var cells []string
		for col := 0; col < g.columns; col++ {
			idx := row*g.columns + col
			if idx >= g.maxWorkers {
				break
			}

			worker := g.workers[idx]
			var cell string
			if worker.active {
				cell = fmt.Sprintf("%d:%s", idx+1, worker.spinner.View())
			} else {
				cell = fmt.Sprintf("%d:○", idx+1)
			}
			cells = append(cells, lipgloss.NewStyle().Width(workerWidth).Align(lipgloss.Center).Render(cell))
		}
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, cells...))
		sb.WriteString("\n")
	}

	return g.style.Width(g.width).Render(sb.String())
}

// ActiveCount returns the number of active workers
func (g *WorkerGrid) ActiveCount() int {
	count := 0
	for _, w := range g.workers {
		if w.active {
			count++
		}
	}
	return count
}

// SetSize updates the grid dimensions
func (g *WorkerGrid) SetSize(width, height int) {
	g.width = width
	g.height = height
	cols := (width - 4) / 12 // Account for borders and spacing
	if cols >= 2 {
		g.columns = cols
	}
}
