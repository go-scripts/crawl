package common

import (
	"time"
	"github.com/briandowns/spinner"
)

// ExtractedContent holds the content extracted from HTML elements
type ExtractedContent struct {
	Text       string            `json:"text"`
	HTML       string            `json:"html"`
	Attributes map[string]string `json:"attributes"`
	Return     string            `json:"return,omitempty"`
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

// ScrapedData represents the data extracted from a crawled page
type ScrapedData struct {
	URL         string                        `json:"url"`
	Title       string                        `json:"title"`
	Selectors   map[string][]ExtractedContent `json:"selectors"`
	Links       []string                      `json:"links"`
	Console     ExtractedContent              `json:"console"`
	StatusCode  int                           `json:"status_code"`
	ContentType string                        `json:"content_type"`
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

