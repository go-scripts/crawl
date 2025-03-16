package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileWriter handles writing scraped data to files
type FileWriter struct {
	outputDir string
	mu        sync.Mutex
}

// NewFileWriter creates a new FileWriter with the specified output directory
func NewFileWriter(outputDir string) (*FileWriter, error) {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating output directory: %v", err)
	}

	return &FileWriter{
		outputDir: outputDir,
	}, nil
}

// WriteURLData writes the scraped data for a URL to a file
func (fw *FileWriter) WriteURLData(data ScrapedData) error {
	// Create a safe filename from URL
	u, err := url.Parse(data.URL)
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%s_%s.json",
		u.Hostname(),
		strings.ReplaceAll(strings.Trim(u.Path, "/"), "/", "_"))

	if filename == "" || filename == "_" {
		filename = "index"
	}

	// Add timestamp to ensure uniqueness
	filename = fmt.Sprintf("%d_%s", time.Now().UnixNano(), filename)

	fullPath := filepath.Join(fw.outputDir, filename)

	// Serialize the data to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	// Lock to prevent concurrent file access issues
	fw.mu.Lock()
	defer fw.mu.Unlock()

	return os.WriteFile(fullPath, jsonData, 0644)
}

