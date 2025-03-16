package main

import (
	"context"
	"sync"
	"time"
)

// ExtractedContent represents data extracted from a single selector
type ExtractedContent struct {
	HTML       string            `json:"html,omitempty"`
	Text       string            `json:"text,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Return     string            `json:"return,omitempty"`
}

// ScrapedData represents the structure of the scraped content
type ScrapedData struct {
	URL        string                        `json:"url"`
	Title      string                        `json:"title"`
	Selectors  map[string][]ExtractedContent `json:"selectors"`
	Links      []string                      `json:"links"`
	Console    ExtractedContent              `json:"console"`
	StatusCode int                           `json:"status_code"`
}

// Configuration holds all the settings for the crawler
type Configuration struct {
	StartURL       string
	Domain         string
	OutputFile     string
	Verbose        bool
	Selectors      []string
	AttributeNames []string
	ExcludePath    []string
	MaxDepth       int
	Concurrency    int
	Timeout        time.Duration
	ExecuteJS      bool
	WaitTime       time.Duration
	Script         string
	UserAgent      string
}

// Crawler manages the website crawling process
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
