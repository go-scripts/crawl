package writer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"localhost:23231/crawl/internal/types"
)

// FileWriter handles writing crawled data to files
type FileWriter struct {
	outputDir string
}

// New creates a new FileWriter instance
func New(outputDir string) (*FileWriter, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}
	return &FileWriter{outputDir: outputDir}, nil
}

// WriteContent writes the extracted content to a file
func (w *FileWriter) WriteContent(content *types.ExtractedContent) error {
	filename := w.sanitizeFilename(content.URL)
	filepath := filepath.Join(w.outputDir, filename+".json")

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(content); err != nil {
		return fmt.Errorf("failed to encode content: %w", err)
	}

	return nil
}

// WriteSummary writes the complete scraped data summary
func (w *FileWriter) WriteSummary(data *types.ScrapedData) error {
	filepath := filepath.Join(w.outputDir, "summary.json")

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create summary file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode summary: %w", err)
	}

	return nil
}

// sanitizeFilename creates a safe filename from a URL
func (w *FileWriter) sanitizeFilename(url string) string {
	// Remove scheme and common prefixes
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "www.")

	// Replace unsafe characters
	unsafe := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", " "}
	for _, char := range unsafe {
		url = strings.ReplaceAll(url, char, "_")
	}

	return url
}
