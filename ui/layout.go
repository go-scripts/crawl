package ui

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ScrapedData represents the data extracted from a crawled page
type ScrapedData struct {
    URL         string                        `json:"url"`
    Title       string                        `json:"title"`
    Links       []string                      `json:"links"`
    StatusCode  int                           `json:"status_code"`
    ContentType string                        `json:"content_type"`
    Selectors   map[string][]ExtractedContent `json:"selectors"`
    Console     ExtractedContent              `json:"console"`
}

// Base component interface
type Component interface {
	Init() tea.Cmd
	Update(tea.Msg) (Component, tea.Cmd)
	View() string
	SetSize(width, height int)
}

// Define common styles
var (
	borderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63"))

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true).
			PaddingLeft(1)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("110"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))
)

// Individual panel components
type WorkerPanel struct {
	viewport viewport.Model
	style    lipgloss.Style
	title    string
	width    int
	height   int
	grid     *WorkerGrid
	workers  []string // For tracking worker info
}

func NewWorkerPanel() *WorkerPanel {
	w := &WorkerPanel{
		title:   "Worker Status",
		style:   borderStyle.Copy().BorderForeground(lipgloss.Color("63")),
		grid:    NewWorkerGrid(10), // Default to 10 workers max
		workers: make([]string, 0),
	}
	w.viewport = viewport.New(0, 0)
	return w
}

func (w *WorkerPanel) Init() tea.Cmd {
	return nil
}

func (w *WorkerPanel) Update(msg tea.Msg) (Component, tea.Cmd) {
	var cmd tea.Cmd
	w.viewport, cmd = w.viewport.Update(msg)
	gridCmd := w.grid.Update(msg)
	return w, tea.Batch(cmd, gridCmd)
}

func (w *WorkerPanel) View() string {
	content := titleStyle.Render(w.title) + "\n\n"
	content += w.grid.View()

	if len(w.workers) > 0 {
		content += "\n\nActive Workers:\n"
		// Show last 3 worker status updates
		start := len(w.workers)
		if start > 3 {
			start = len(w.workers) - 3
		}
		for _, worker := range w.workers[start:] {
			content += infoStyle.Render("• " + worker + "\n")
		}
	}

	w.viewport.SetContent(content)
	return w.style.Width(w.width).Height(w.height).Render(w.viewport.View())
}

func (w *WorkerPanel) SetSize(width, height int) {
	w.width = width
	w.height = height
	w.viewport.Width = width - 4 // Account for borders
	w.viewport.Height = height - 4
	w.grid.SetSize(width-4, height-6)
}

// AddWorker adds a worker to the grid and updates worker info
func (w *WorkerPanel) AddWorker(workerInfo string) {
	w.workers = append(w.workers, workerInfo)
	// Activate a worker in the grid
	activeCount := w.grid.ActiveCount()
	if activeCount < 10 { // Max workers
		w.grid.ActivateWorker(activeCount, workerInfo)
	}
}

// RemoveWorker deactivates a worker by ID
func (w *WorkerPanel) RemoveWorker(workerID int) {
	if workerID >= 0 && workerID < 10 {
		w.grid.DeactivateWorker(workerID)
	}
}

type QueuePanel struct {
	viewport viewport.Model
	style    lipgloss.Style
	title    string
	width    int
	height   int
	queue    *QueueList
}

func NewQueuePanel() *QueuePanel {
	q := &QueuePanel{
		title: "Queue Management",
		style: borderStyle.Copy().BorderForeground(lipgloss.Color("99")),
		queue: NewQueueList(),
	}
	return q
}

func (q *QueuePanel) Init() tea.Cmd {
	return nil
}

func (q *QueuePanel) Update(msg tea.Msg) (Component, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			q.queue.list.CursorUp()
			return q, nil
		case "down", "j":
			q.queue.list.CursorDown()
			return q, nil
		}
	}

	queueCmd := q.queue.Update(msg)
	return q, tea.Batch(cmd, queueCmd)
}

func (q *QueuePanel) View() string {
	return q.style.Width(q.width).Height(q.height).Render(q.queue.View())
}

func (q *QueuePanel) SetSize(width, height int) {
	q.width = width
	q.height = height
	q.queue.SetSize(width-4, height-4)
}

// AddToQueue adds a URL to the queue
func (q *QueuePanel) AddToQueue(url string) {
	q.queue.AddURL(url, 1) // Default priority 1
}

// MarkURLProcessed marks a URL as processed
func (q *QueuePanel) MarkURLProcessed(url string) {
	q.queue.MarkURLProcessed(url)
}

// Clear empties the queue
func (q *QueuePanel) Clear() {
	q.queue.Clear()
}

type ResultsPanel struct {
	viewport viewport.Model
	style    lipgloss.Style
	title    string
	width    int
	height   int
	table    *ResultsTable
}

func NewResultsPanel() *ResultsPanel {
	r := &ResultsPanel{
		title: "Crawl Results",
		style: borderStyle.Copy().BorderForeground(lipgloss.Color("35")),
		table: NewResultsTable(),
	}
	return r
}

func (r *ResultsPanel) Init() tea.Cmd {
	return nil
}

func (r *ResultsPanel) Update(msg tea.Msg) (Component, tea.Cmd) {
	cmd := r.table.Update(msg)
	return r, cmd
}

func (r *ResultsPanel) View() string {
	return r.style.Width(r.width).Height(r.height).Render(r.table.View())
}

func (r *ResultsPanel) SetSize(width, height int) {
	r.width = width
	r.height = height
	r.table.SetSize(width-4, height-4)
}

func (r *ResultsPanel) AddResult(result CrawlResult) {
	r.table.AddResult(result)
}

type ErrorPanel struct {
	viewport viewport.Model
	style    lipgloss.Style
	title    string
	width    int
	height   int
	console  *ErrorConsole
}

func NewErrorPanel() *ErrorPanel {
	e := &ErrorPanel{
		title:   "Error/Warning Console",
		style:   borderStyle.Copy().BorderForeground(lipgloss.Color("196")),
		console: NewErrorConsole(),
	}
	return e
}

func (e *ErrorPanel) Init() tea.Cmd {
	return nil
}

func (e *ErrorPanel) Update(msg tea.Msg) (Component, tea.Cmd) {
	cmd := e.console.Update(msg)
	return e, cmd
}

func (e *ErrorPanel) View() string {
	return e.style.Width(e.width).Height(e.height).Render(e.console.View())
}

func (e *ErrorPanel) SetSize(width, height int) {
	e.width = width
	e.height = height
	e.console.SetSize(width-4, height-4)
}

// Log message adding methods
func (e *ErrorPanel) AddError(msg string) {
	e.console.AddEntry(LevelError, msg)
}

func (e *ErrorPanel) AddWarning(msg string) {
	e.console.AddEntry(LevelWarning, msg)
}

func (e *ErrorPanel) AddInfo(msg string) {
	e.console.AddEntry(LevelInfo, msg)
}

// Layout manager
type Layout struct {
	workers Component
	queue   Component
	results Component
	errors  Component
	stats   *StatsPanel // Use concrete type for direct access
	width   int
	height  int
}

// NewLayout creates and initializes a new layout with all panels
func NewLayout() *Layout {
	return &Layout{
		workers: NewWorkerPanel(),
		queue:   NewQueuePanel(),
		results: NewResultsPanel(),
		errors:  NewErrorPanel(),
		stats:   NewStatsPanel(),
	}
}

// SetSize adjusts the layout and all components to the given dimensions
func (l *Layout) SetSize(width, height int) {
	l.width = width
	l.height = height

	// Calculate panel dimensions
	halfWidth := width / 2
	halfHeight := height / 2

	// Workers and Stats share the left side
	workerHeight := int(float64(halfHeight) * 0.6)
	statsHeight := halfHeight - workerHeight

	l.workers.SetSize(halfWidth, workerHeight)
	l.stats.SetSize(halfWidth, statsHeight)
	l.queue.SetSize(halfWidth, halfHeight)
	l.results.SetSize(halfWidth, halfHeight)
	l.errors.SetSize(width, height-halfHeight)
}

// Init initializes all panels
func (l *Layout) Init() tea.Cmd {
	return tea.Batch(
		l.workers.Init(),
		l.queue.Init(),
		l.results.Init(),
		l.errors.Init(),
	)
}

// Update processes messages and updates components
func (l *Layout) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.SetSize(msg.Width, msg.Height)
	}

	var cmd tea.Cmd
	l.workers, cmd = l.workers.Update(msg)
	cmds = append(cmds, cmd)

	l.queue, cmd = l.queue.Update(msg)
	cmds = append(cmds, cmd)

	l.results, cmd = l.results.Update(msg)
	cmds = append(cmds, cmd)

	l.errors, cmd = l.errors.Update(msg)
	cmds = append(cmds, cmd)

	// Update stats panel
	statsCmd := l.stats.Update(msg)
	cmds = append(cmds, statsCmd)

	return l, tea.Batch(cmds...)
}

// View renders the complete layout
func (l *Layout) View() string {
	// Top left: workers and stats stacked
	leftSide := lipgloss.JoinVertical(
		lipgloss.Left,
		l.workers.View(),
		l.stats.View(),
	)

	// Top row: left side and queue
	topRow := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftSide,
		l.queue.View(),
	)

	// Stack all components
	return lipgloss.JoinVertical(
		lipgloss.Left,
		topRow,
		l.results.View(),
		l.errors.View(),
	)
}

// AddWorker adds a worker to the worker panel
func (l *Layout) AddWorker(workerInfo string) {
	if wp, ok := l.workers.(*WorkerPanel); ok {
		wp.AddWorker(workerInfo)
	}
}

// RemoveWorker removes a worker from the grid
func (l *Layout) RemoveWorker(workerID int) {
	if wp, ok := l.workers.(*WorkerPanel); ok {
		wp.RemoveWorker(workerID)
	}
}

// AddToQueue adds a URL to the queue panel
func (l *Layout) AddToQueue(url string) {
	if qp, ok := l.queue.(*QueuePanel); ok {
		qp.AddToQueue(url)
	}
}

// MarkURLProcessed marks a URL as processed in the queue
func (l *Layout) MarkURLProcessed(url string) {
	if qp, ok := l.queue.(*QueuePanel); ok {
		qp.MarkURLProcessed(url)
	}
}

// ClearQueue empties the queue panel
func (l *Layout) ClearQueue() {
	if qp, ok := l.queue.(*QueuePanel); ok {
		qp.Clear()
	}
}

// AddResult adds a crawl result to the results panel
func (l *Layout) AddResult(result CrawlResult) {
	if rp, ok := l.results.(*ResultsPanel); ok {
		rp.AddResult(result)
	}
}

type ExtractedContent struct {
	HTML       string            `json:"html,omitempty"`
	Text       string            `json:"text,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Return     string            `json:"return,omitempty"`
}

// AddCrawlResult is a convenience method that creates a CrawlResult from scraped data
func (l *Layout) AddCrawlResult(data ScrapedData, err error) {
	result := CrawlResult{
		URL:        data.URL,
		Title:      data.Title,
		StatusCode: data.StatusCode,
		Links:      len(data.Links),
		Type:       data.ContentType,
	}
	if err != nil {
		result.Error = err.Error()
	}
	l.AddResult(result)
}

// AddError adds an error message to the error console
func (l *Layout) AddError(msg string) {
	if ep, ok := l.errors.(*ErrorPanel); ok {
		ep.AddError(msg)
	}
}

// AddWarning adds a warning message to the error console
func (l *Layout) AddWarning(msg string) {
	if ep, ok := l.errors.(*ErrorPanel); ok {
		ep.AddWarning(msg)
	}
}

// AddInfo adds an info message to the error console
func (l *Layout) AddInfo(msg string) {
	if ep, ok := l.errors.(*ErrorPanel); ok {
		ep.AddInfo(msg)
	}
}

// Log adds a message with the specified level to the error console
func (l *Layout) Log(msg string, level LogLevel) {
	if ep, ok := l.errors.(*ErrorPanel); ok {
		ep.console.AddEntry(level, msg)
	}
}

// Add methods to update statistics
func (l *Layout) UpdateStats(stats CrawlStats) {
	l.stats.UpdateStats(stats)
}

func (l *Layout) AddProcessedURL(url string) {
	l.stats.AddProcessedURL(url)
}
