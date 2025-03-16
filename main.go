package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	"github.com/briandowns/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/go-scripts/crawl/pkg/crawl"
	"github.com/go-scripts/crawl/ui"
)

// ExtractedContent holds the content extracted from HTML elements
type ExtractedContent struct {
	Text       string            `json:"text"`
	HTML       string            `json:"html"`
	Attributes map[string]string `json:"attributes"`
}
// Configuration holds the crawler configuration
type Configuration struct {
	StartURL       string
	MaxDepth       int
	Concurrency    int
	UserAgent      string
	OutputFile     string
	Domain         string
	Verbose        bool
	ExecuteJS      bool
	WaitTime       time.Duration
	Timeout        time.Duration
	Selectors      []string
	AttributeNames []string
	ExcludePath    []string
	Script         string
}

// ScrapedData is defined in models.go

// Crawler represents the web crawler
type Crawler struct {
	config        Configuration
	visited       map[string]bool
	queue         Queue
	results       []ScrapedData
	visitedMu     sync.Mutex
	resultsMu     sync.Mutex
	wg            sync.WaitGroup
	browserCtx    context.Context
	browserCancel context.CancelFunc
	fileWriter    *FileWriter
}

// Queue represents the crawler's work queue
type Queue struct {
	spinners      []*spinner.Spinner
	spinnerStatus []bool
	spinnerURLs   []string
}

// FileWriter handles writing crawled data to files
type FileWriter struct {
	outputDir string
}

// NewFileWriter creates a new FileWriter instance
func NewFileWriter(outputDir string) (*FileWriter, error) {
	return &FileWriter{
		outputDir: outputDir,
	}, nil
}

// WriteURLData writes the scraped data for a URL to a file
func (fw *FileWriter) WriteURLData(data ScrapedData) error {
	// Placeholder for actual file writing implementation
	return nil
}

// CLI flags structure
type CLIFlags struct {
	// Preserve existing CLI flags
	ConfigFile  string `help:"Path to configuration file" default:"config.yaml"`
	StartURL    string `help:"Starting URL for the crawler" short:"u"`
	MaxDepth    int    `help:"Maximum crawl depth" default:"3" short:"d"`
	Concurrency int    `help:"Number of concurrent workers" default:"5" short:"c"`
	OutputFile  string `help:"Path to output file" default:"results.json" short:"o"`
	Debug       bool   `help:"Enable debug mode" default:"false"`
}

// Base model structure
type Model struct {
	config  *Configuration
	flags   CLIFlags
	crawler *Crawler
	ready   bool
	err     error
	layout  *ui.Layout

	// TUI state fields
	workerStates  map[int]bool // Track individual worker states
	activeWorkers int
	queueSize     int
	crawledPages  []ScrapedData
	errors        []string
	startTime     time.Time

	// Statistics tracking
	successCount    int
	failCount       int
	totalTime       time.Duration
	responseTimeSum time.Duration
	lastUpdate      time.Time
}

// Add helper methods for timing
func (m *Model) elapsedTime() string {
	if m.startTime.IsZero() {
		return "00:00:00"
	}
	elapsed := time.Since(m.startTime)
	return fmt.Sprintf("%02d:%02d:%02d",
		int(elapsed.Hours()),
		int(elapsed.Minutes())%60,
		int(elapsed.Seconds())%60,
	)
}

// Count the number of pages with error status codes
func (m *Model) countErrors() int {
	count := 0
	for _, page := range m.crawledPages {
		if page.StatusCode >= 400 {
			count++
		}
	}
	return count
}

// Message types
type CrawlerStartedMsg struct{}
type CrawlerFinishedMsg struct{}
type PageCrawledMsg struct {
	data      ScrapedData
	err       error
	startTime time.Time
}
type ErrorMsg struct {
	err error
}
type WorkerUpdateMsg struct {
	workerID int
	active   bool
	url      string
}
type QueueUpdateMsg struct {
	url    string
	action string // "add" or "complete"
}

// Stats ticker message
type statsTickMsg struct{}

// Function to return stats tick message
func tickStats() tea.Msg {
	return statsTickMsg{}
}

// Init is the first function called. It returns an optional initial command.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.layout.Init(),
		tea.Every(time.Second, func(t time.Time) tea.Msg {
			return statsTickMsg{}
		}),
	)
}

// updateStats updates the statistics display
func (m *Model) updateStats() {
	stats := ui.CrawlStats{
		TotalURLs:      len(m.crawledPages) + m.queueSize,
		ProcessedURLs:  len(m.crawledPages),
		QueuedURLs:     m.queueSize,
		SuccessfulURLs: m.successCount,
		FailedURLs:     m.failCount,
		ActiveWorkers:  m.activeWorkers,
		StartTime:      m.startTime,
	}

	// Calculate average response time if we have processed pages
	if len(m.crawledPages) > 0 {
		stats.AverageTime = m.responseTimeSum / time.Duration(len(m.crawledPages))
	}

	// Update last URLs list
	if len(m.crawledPages) > 0 {
		end := len(m.crawledPages)
		start := end - 5
		if start < 0 {
			start = 0
		}
		stats.LastProcessedURLs = make([]string, 0, end-start)
		for _, page := range m.crawledPages[start:end] {
			stats.LastProcessedURLs = append(stats.LastProcessedURLs, page.URL)
		}
	}

	m.layout.UpdateStats(stats)
	m.lastUpdate = time.Now()
}


// Update handles all the updates and state transitions
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Create a slice to track all commands
	var cmds []tea.Cmd

	// Process message based on type
	switch msg := msg.(type) {
	case statsTickMsg:
		m.updateStats()
		cmds = append(cmds, tickStats)

	case tea.WindowSizeMsg:
		// Handle window size
		m.layout.SetSize(msg.Width, msg.Height)

	case tea.KeyMsg:
		// Handle keyboard input
		switch msg.String() {
		case "ctrl+c", "q":
			m.layout.AddInfo("Shutting down crawler...")
			return m, tea.Quit
		}

	case CrawlerStartedMsg:
		// Handle crawler started message with enhanced logging
		m.startTime = time.Now()
		m.layout.AddInfo("Crawler started")
		m.layout.AddInfo(fmt.Sprintf("Configuration: max depth=%d, concurrent workers=%d",
			m.config.MaxDepth, m.config.Concurrency))
		m.layout.AddResult(ui.CrawlResult{
			URL:  "Crawler Started",
			Type: "system",
		})

	case CrawlerFinishedMsg:
		// Handle crawler finished message with timing information
		m.layout.AddInfo(fmt.Sprintf("Crawler finished. Total time: %s", m.elapsedTime()))
		m.layout.AddInfo(fmt.Sprintf("Processed %d pages, %d errors",
			len(m.crawledPages),
			m.countErrors()))
		m.layout.AddResult(ui.CrawlResult{
			URL:  "Crawler Finished",
			Type: "system",
		})

	case PageCrawledMsg:
		// Handle page crawled message with enhanced status reporting
		m.crawledPages = append(m.crawledPages, msg.data)

		// Track response time
		responseTime := time.Since(msg.startTime)
		m.responseTimeSum += responseTime
		m.totalTime += responseTime

		if msg.err != nil {
			m.failCount++
			m.errors = append(m.errors, msg.err.Error())
			m.layout.AddError(fmt.Sprintf("Error crawling %s: %v [%v]",
				msg.data.URL, msg.err, responseTime.Round(time.Millisecond)))
		} else {
			if msg.data.StatusCode < 400 {
				m.successCount++
				m.layout.AddInfo(fmt.Sprintf("Successfully crawled %s (status: %d, links: %d) in %v",
					msg.data.URL, msg.data.StatusCode, len(msg.data.Links), responseTime.Round(time.Millisecond)))
			} else {
				m.failCount++
				m.layout.AddWarning(fmt.Sprintf("Failed to crawl %s (status: %d) in %v",
					msg.data.URL, msg.data.StatusCode, responseTime.Round(time.Millisecond)))
			}
		}
		
		// Convert ScrapedData to ui.ScrapedData
		uiScrapedData := ui.ScrapedData{
			URL:         msg.data.URL,
			Title:       msg.data.Title,
			Selectors:   msg.data.Selectors,
			Links:       msg.data.Links,
			Console:     msg.data.Console,
			StatusCode:  msg.data.StatusCode,
			ContentType: msg.data.ContentType,
		}
		m.layout.AddCrawlResult(uiScrapedData, msg.err)
		m.layout.AddProcessedURL(msg.data.URL)
		m.updateStats()

	case ErrorMsg:
		// Handle error message
		m.err = msg.err
		m.errors = append(m.errors, msg.err.Error())
		m.layout.AddError(msg.err.Error())

	case WorkerUpdateMsg:
		// Handle worker update message with more detailed logging
		if msg.active {
			m.workerStates[msg.workerID] = true
			m.activeWorkers++
			m.layout.AddInfo(fmt.Sprintf("Worker %d started processing %s",
				msg.workerID, msg.url))
			m.layout.AddWorker(fmt.Sprintf("Worker %d: %s", msg.workerID, msg.url))
		} else {
			delete(m.workerStates, msg.workerID)
			m.activeWorkers--
			m.layout.AddInfo(fmt.Sprintf("Worker %d finished",
				msg.workerID))
			m.layout.RemoveWorker(msg.workerID)
		}

	case QueueUpdateMsg:
		// Handle queue update message with more detailed logging
		switch msg.action {
		case "add":
			m.queueSize++
			m.layout.AddInfo(fmt.Sprintf("Queued: %s", msg.url))
			m.layout.AddToQueue(msg.url)
		case "complete":
			m.queueSize--
			m.layout.AddInfo(fmt.Sprintf("Completed: %s", msg.url))
			m.layout.MarkURLProcessed(msg.url)
		}
		m.updateStats()
	}

	// Update the layout with the message
	layoutModel, layoutCmd := m.layout.Update(msg)
	if updatedLayout, ok := layoutModel.(*ui.Layout); ok {
		m.layout = updatedLayout
	}
	cmds = append(cmds, layoutCmd)

	return m, tea.Batch(cmds...)
}

// View returns a string representation of the UI
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress any key to quit.", m.err)
	}

	if !m.ready {
		return "Initializing...\n"
	}

	return m.layout.View()
}

// loadConfig loads the configuration from the specified file
func loadConfig(path string) (*Configuration, error) {
	// Placeholder for actual configuration loading
	return &Configuration{
		StartURL:    "https://example.com",
		MaxDepth:    3,
		Concurrency: 5,
		UserAgent:   "WebCrawler/1.0",
		OutputFile:  "results.json",
	}, nil
}

// createCrawler creates a new crawler with the given configuration
func createCrawler(config *Configuration) (*Crawler, error) {
	// Create and return a new crawler with the provided configuration
	return crawl.NewCrawler(*config)
}

func main() {
	var flags CLIFlags

	// Parse command line flags using kong
	ctx := kong.Parse(&flags)
	if ctx.Error != nil {
		fmt.Printf("Error parsing flags: %v\n", ctx.Error)
		os.Exit(1)
	}

	// Load configuration from file
	config, err := loadConfig(flags.ConfigFile)
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Override config with command line flags if provided
	if flags.StartURL != "" {
		config.StartURL = flags.StartURL
	}
	if flags.MaxDepth != 0 {
		config.MaxDepth = flags.MaxDepth
	}
	if flags.Concurrency != 0 {
		config.Concurrency = flags.Concurrency
	}
	if flags.OutputFile != "" {
		config.OutputFile = flags.OutputFile
	}

	// Create crawler
	crawler, err := createCrawler(config)
	if err != nil {
		fmt.Printf("Error creating crawler: %v\n", err)
		os.Exit(1)
	}
	// Initialize model
	model := Model{
		config:  config,
		flags:   flags,
		crawler: crawler,
		ready:   true,
		// Initialize the layout
		layout:          ui.NewLayout(),
		workerStates:    make(map[int]bool),
		activeWorkers:   0,
		queueSize:       0,
		crawledPages:    make([]ScrapedData, 0),
		startTime:       time.Time{},
		successCount:    0,
		failCount:       0,
		totalTime:       0,
		responseTimeSum: 0,
		lastUpdate:      time.Now(),
		errors:          make([]string, 0),
	}
	// Run the Bubble Tea program
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
