package crawler

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"localhost:23231/crawl/internal/progress"
	"localhost:23231/crawl/internal/queue"
	"localhost:23231/crawl/internal/types"
	"localhost:23231/crawl/internal/writer"

	"golang.org/x/net/html"
)

// Configuration holds the crawler settings
type Configuration struct {
	StartURL    string
	MaxDepth    int
	OutputDir   string
	Concurrency int
}

// Crawler manages the web crawling process
type Crawler struct {
	config   Configuration
	queue    *queue.Queue
	writer   *writer.FileWriter
	progress *progress.ProgressTracker
	client   *http.Client
	wg       sync.WaitGroup
}

// New creates a new Crawler instance
func New(config Configuration) (*Crawler, error) {
	q := queue.New()
	w, err := writer.New(config.OutputDir)
	if err != nil {
		return nil, err
	}

	return &Crawler{
		config:   config,
		queue:    q,
		writer:   w,
		progress: progress.New(),
		client:   &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Start begins the crawling process
func (c *Crawler) Start() error {
	// Add the starting URL
	c.queue.Add(c.config.StartURL)

	// Launch worker goroutines
	for i := 0; i < c.config.Concurrency; i++ {
		c.wg.Add(1)
		go c.worker()
	}

	// Wait for all workers to finish
	c.wg.Wait()

	return nil
}

// worker processes URLs from the queue
func (c *Crawler) worker() {
	defer c.wg.Done()

	for {
		url, ok := c.queue.Next()
		if !ok {
			return
		}

		c.progress.StartProcessingPage(url)
		content, err := c.crawlPage(url)
		if err != nil {
			fmt.Printf("\nError crawling %s: %v", url, err)
			continue
		}

		if err := c.writer.WriteContent(content); err != nil {
			fmt.Printf("\nError writing %s: %v", url, err)
		}

		c.progress.FinishProcessingPage(url)
	}
}

// crawlPage fetches and processes a single page
func (c *Crawler) crawlPage(url string) (*types.ExtractedContent, error) {
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	content := &types.ExtractedContent{
		URL:   url,
		Title: extractTitle(doc),
		Links: extractLinks(doc),
	}

	return content, nil
}

// Helper functions for HTML parsing
func extractTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" {
		if n.FirstChild != nil {
			return n.FirstChild.Data
		}
		return ""
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := extractTitle(c); title != "" {
			return title
		}
	}
	return ""
}

func extractLinks(n *html.Node) []string {
	var links []string

	if n.Type == html.ElementNode && n.Data == "a" {
		for _, a := range n.Attr {
			if a.Key == "href" {
				links = append(links, a.Val)
				break
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		links = append(links, extractLinks(c)...)
	}

	return links
}
