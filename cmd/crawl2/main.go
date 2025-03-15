package main

import (
    "flag"
    "log"
    "os"
    
    "localhost:23231/crawl/internal/crawler"
    "localhost:23231/crawl/internal/progress"
)

func main() {
    // Flag definitions
    startURL := flag.String("url", "", "The starting URL to crawl")
    maxDepth := flag.Int("depth", 2, "Maximum crawl depth")
    outputDir := flag.String("output", "output", "Directory to store output files")
    concurrency := flag.Int("concurrency", 2, "Number of concurrent crawlers")
    
    flag.Parse()
    
    if *startURL == "" {
        log.Fatal("URL is required")
    }
    
    // Create crawler configuration
    config := crawler.Configuration{
        StartURL:    *startURL,
        MaxDepth:    *maxDepth,
        OutputDir:   *outputDir,
        Concurrency: *concurrency,
    }
    
    // Initialize crawler
    c, err := crawler.New(config)
    if err != nil {
        log.Fatalf("Failed to initialize crawler: %v", err)
    }
    
    // Start crawling
    if err := c.Start(); err != nil {
        log.Fatalf("Crawling failed: %v", err)
    }
}

package main

import (
	"flag"
	"log"
	"os"
	
	// Internal package imports will go here
	// "github.com/charmbracelet/bubbles/progress"
	// "../internal/crawler"
	// "../internal/queue"
	// "../internal/writer"
	// "../internal/types"
	// "../internal/progress"
)

func main() {
	// Flag definitions
	startURL := flag.String("url", "", "The starting URL to crawl")
	maxDepth := flag.Int("depth", 2, "Maximum crawl depth")
	outputDir := flag.String("output", "output", "Directory to store output files")
	concurrency := flag.Int("concurrency", 2, "Number of concurrent crawlers")
	
	flag.Parse()
	
	if *startURL == "" {
		log.Fatal("URL is required")
	}
	
	// Main program logic will go here
}

