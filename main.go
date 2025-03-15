package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/charmbracelet/log"
	"github.com/chromedp/chromedp"
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
	URL       string                        `json:"url"`
	Title     string                        `json:"title"`
	Selectors map[string][]ExtractedContent `json:"selectors"`
	Links     []string                      `json:"links"`
	Console   ExtractedContent              `json:"console"`
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
}

// Queue for URLs to be processed
type Queue struct {
	items         []string
	mu            sync.Mutex
	spinners      []*spinner.Spinner
	spinnerStatus []bool   // true if in use
	spinnerURLs   []string // URL associated with each spinner
	spinnerMu     sync.Mutex
}

// FileWriter handles writing scraped data to files
type FileWriter struct {
	outputDir string
	mu        sync.Mutex
}

func (q *Queue) HandleError(urlStr string, err error, errorMessage string) {
	// Skip JavaScript execution errors to reduce noise
	if strings.Contains(strings.ToLower(errorMessage), "script") ||
		strings.Contains(strings.ToLower(errorMessage), "javascript") ||
		strings.Contains(strings.ToLower(errorMessage), "execute") {
		return
	}

	if errorMessage == "" {
		errorMessage = "Error"
	}

	// Create a clean error message with context
	fullMessage := fmt.Sprintf("Error processing %s: %s (%v)",
		formatSpinnerMessage(urlStr),
		errorMessage,
		err)

	fmt.Printf("\n%s\n", fullMessage)
	log.Error(fullMessage)

	// Show condensed error in spinner
	shortMsg := fmt.Sprintf("ERROR: %s", errorMessage)
	if len(shortMsg) > 50 {
		shortMsg = shortMsg[:47] + "..."
	}
	q.UpdateSpinnerMessage(urlStr, shortMsg)
}
func (q *Queue) UpdateSpinnerMessage(urlStr string, message string) {
	q.spinnerMu.Lock()
	defer q.spinnerMu.Unlock()

	for i, spinnerURL := range q.spinnerURLs {
		if spinnerURL == urlStr {
			// Format: [ID] URL (truncated) STATUS
			msg := fmt.Sprintf(" [%d] %s", i, formatSpinnerMessage(urlStr))
			if message != "" {
				msg += fmt.Sprintf("\n    → %s", message)
			}
			q.spinners[i].Suffix = msg
			break
		}
	}
}

func (q *Queue) Push(url string) {
	q.mu.Lock()
	q.items = append(q.items, url)
	q.mu.Unlock()

	q.spinnerMu.Lock()
	defer q.spinnerMu.Unlock()

	// Find an available spinner
	spinnerID := -1
	for i, inUse := range q.spinnerStatus {
		if !inUse {
			spinnerID = i
			break
		}
	}

	if spinnerID != -1 {
		q.spinnerStatus[spinnerID] = true
		q.spinnerURLs[spinnerID] = url
		q.spinners[spinnerID].Suffix = fmt.Sprintf(" [%d] %s", spinnerID, formatSpinnerMessage(url))
		q.spinners[spinnerID].Start()
	} else if len(q.spinnerStatus) > 0 {
		// If no spinner is available, use the first one
		spinnerID = 0
		q.spinnerURLs[spinnerID] = url
		q.spinners[spinnerID].Suffix = fmt.Sprintf(" [%d] %s", spinnerID, formatSpinnerMessage(url))
	}
}

func NewFileWriter(outputDir string) (*FileWriter, error) {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating output directory: %v", err)
	}

	return &FileWriter{
		outputDir: outputDir,
	}, nil
}

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

// Helper method for formatting URLs in spinner messages
func formatSpinnerMessage(urlStr string) string {
	// Truncate URL if too long
	maxLen := 40
	if len(urlStr) > maxLen {
		// Keep the protocol and domain, then truncate the path
		u, err := url.Parse(urlStr)
		if err == nil {
			domain := u.Host
			path := u.Path
			if len(path) > maxLen-len(domain)-3 {
				path = "..." + path[len(path)-(maxLen-len(domain)-3):]
			}
			return domain + path
		}
		return "..." + urlStr[len(urlStr)-maxLen:]
	}
	return urlStr
}

func (q *Queue) Pop() (string, bool) {
	q.mu.Lock()
	if len(q.items) == 0 {
		q.mu.Unlock()
		return "", false
	}

	item := q.items[0]
	q.items = q.items[1:]
	q.mu.Unlock()

	q.spinnerMu.Lock()
	defer q.spinnerMu.Unlock()

	// Find and release the spinner
	for i, url := range q.spinnerURLs {
		if url == item {
			q.spinners[i].Stop()
			q.spinnerStatus[i] = false
			q.spinnerURLs[i] = ""
			break
		}
	}

	return item, true
}

func (q *Queue) IsEmpty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) == 0
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

func NewCrawler(config Configuration) (*Crawler, error) {
	// Setup browser options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.NoSandbox,
		chromedp.Headless,
	)

	// Create a browser context that will be shared across all crawler operations
	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)

	// Setup file writer
	fileWriter, err := NewFileWriter(filepath.Dir(config.OutputFile))
	if err != nil {
		return nil, err
	}
	// Initialize spinners based on concurrency
	spinners := make([]*spinner.Spinner, config.Concurrency)
	spinnerStatus := make([]bool, config.Concurrency)
	spinnerURLs := make([]string, config.Concurrency)

	for i := 0; i < config.Concurrency; i++ {
		spinners[i] = spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	}

	return &Crawler{
		config:  config,
		visited: make(map[string]bool),
		queue: Queue{
			spinners:      spinners,
			spinnerStatus: spinnerStatus,
			spinnerURLs:   spinnerURLs,
		},
		browserCtx:    browserCtx,
		browserCancel: browserCancel,
		fileWriter:    fileWriter,
	}, nil
}

func (c *Crawler) Close() {
	c.browserCancel()
}

func (c *Crawler) isVisited(urlStr string) bool {
	c.visitedMu.Lock()
	defer c.visitedMu.Unlock()
	return c.visited[urlStr]
}

func (c *Crawler) markVisited(urlStr string) {
	c.visitedMu.Lock()
	defer c.visitedMu.Unlock()
	c.visited[urlStr] = true
}

func (c *Crawler) addResult(data ScrapedData) {
	c.resultsMu.Lock()
	defer c.resultsMu.Unlock()
	c.results = append(c.results, data)
}

func (c *Crawler) isSameDomain(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return u.Hostname() == c.config.Domain
}

func (c *Crawler) shouldExclude(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return true
	}

	path := u.Path

	for _, exclude := range c.config.ExcludePath {
		if strings.Contains(path, exclude) {
			return true
		}
	}

	return false
}

func (c *Crawler) normalizeURL(baseURL, href string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	reference, err := url.Parse(href)
	if err != nil {
		return "", err
	}

	return base.ResolveReference(reference).String(), nil
}

// getAttributes extracts attributes from an element using JavaScript
func getAttributesJS(attributeNames []string) string {
	// if len(attributeNames) == 0 {
	// 	// Default common attributes
	// 	attributeNames = []string{"id", "class", "href", "src", "alt", "title"}
	// }

	attrNamesJSON, _ := json.Marshal(attributeNames)

	return fmt.Sprintf(`
    function(el) {
        const result = {};
        const attrs = %s;

        // Handle case where no attributes are specified
        if (!attrs || attrs.length === 0) {
            // Get all attributes, but first check if element has attributes
            if (el && el.attributes) {
                for (let i = 0; i < el.attributes.length; i++) {
                    const attr = el.attributes[i];
                    result[attr.name] = attr.value;
                }
            }
        } else {
            // Get only specified attributes
            for (const attrName of attrs) {
                if (el && el.hasAttribute(attrName)) {
                    result[attrName] = el.getAttribute(attrName);
                }
            }
        }

        return result;
    }`, attrNamesJSON)
}

func (c *Crawler) scrape(targetURL string) {
	defer c.wg.Done()

	if c.isVisited(targetURL) {
		return
	}

	c.markVisited(targetURL)

	// Create a context for this browser tab
	tabCtx, cancel := chromedp.NewContext(c.browserCtx)
	defer cancel()

	// Add timeout
	timeoutCtx, timeoutCancel := context.WithTimeout(tabCtx, c.config.Timeout)
	defer timeoutCancel()

	// Variables to store page data
	var pageTitle string
	var pageHTML string
	var links []string
	selectorResults := make(map[string][]ExtractedContent)

	// Navigate to page and wait for it to load
	tasks := []chromedp.Action{
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body"),
	}

	// Add wait time if specified
	if c.config.WaitTime > 0 {
		tasks = append(tasks, chromedp.Sleep(c.config.WaitTime))
	}

	// Execute navigation
	if err := chromedp.Run(timeoutCtx, tasks...); err != nil {
		c.queue.HandleError(targetURL, err, "Navigation failed")
		return
	}

	// Get page title
	if err := chromedp.Run(timeoutCtx, chromedp.Title(&pageTitle)); err != nil {
		c.queue.HandleError(targetURL, err, "Error getting title")
		// Continue anyway, title is not critical
	}

	// Get full page HTML
	if err := chromedp.Run(timeoutCtx, chromedp.OuterHTML("html", &pageHTML)); err != nil {
		c.queue.HandleError(targetURL, err, "Error getting HTML")
		return
	}

	// Process each selector
	for _, selector := range c.config.Selectors {
		var extractedContents []ExtractedContent

		// JavaScript for finding elements and extracting content
		var nodesJSON string
		jsSelector := fmt.Sprintf(`
		(() => {
			const elements = document.querySelectorAll(%q);
			const getAttrs = %s;
			const results = [];
			
			elements.forEach(el => {
				results.push({
					html: el.innerHTML,
					text: el.textContent.trim(),
					attributes: getAttrs(el)
				});
			});
			
			return JSON.stringify(results);
		})()
		`, selector, getAttributesJS(c.config.AttributeNames))

		err := chromedp.Run(timeoutCtx, chromedp.Evaluate(jsSelector, &nodesJSON))

		if err != nil {
			c.queue.HandleError(targetURL, err, fmt.Sprintf("Error executing query for selector %s", selector))
			continue
		}

		// Parse results
		var extractedData []map[string]interface{}
		if err := json.Unmarshal([]byte(nodesJSON), &extractedData); err != nil {
			c.queue.HandleError(targetURL, err, fmt.Sprintf("Error parsing selector results for %s", selector))
			continue
		}

		// Convert to our structure
		for _, data := range extractedData {
			content := ExtractedContent{
				HTML: data["html"].(string),
				Text: data["text"].(string),
			}

			// Extract attributes
			if attrs, ok := data["attributes"].(map[string]interface{}); ok {
				content.Attributes = make(map[string]string)
				for key, val := range attrs {
					content.Attributes[key] = val.(string)
				}
			}

			extractedContents = append(extractedContents, content)
		}

		if len(extractedContents) > 0 {
			selectorResults[selector] = extractedContents
		}
	}

	var extractedConsole ExtractedContent
	if c.config.Script != "" {
		var nodesScriptJSON string
		err := chromedp.Run(timeoutCtx, chromedp.Evaluate(c.config.Script, &nodesScriptJSON))
		if err != nil {
			c.queue.HandleError(targetURL, err, fmt.Sprintf("Error executing script %s", c.config.Script))
		} else {
			extractedConsole = ExtractedContent{
				Return: nodesScriptJSON,
			}
		}
	}

	// Extract links
	jsLinks := `
	(() => {
		const links = Array.from(document.querySelectorAll('a'));
		return links.map(a => a.href).filter(href => 
			href && 
			!href.startsWith('javascript:') && 
			!href.startsWith('mailto:') && 
			!href.startsWith('tel:') &&
			!href.startsWith('#')
		);
	})()
	`

	err := chromedp.Run(timeoutCtx, chromedp.Evaluate(jsLinks, &links))
	if err != nil {
		c.queue.HandleError(targetURL, err, "Error extracting links")
		// Continue with no links
		links = []string{}
	}

	// Filter links
	var filteredLinks []string
	for _, link := range links {
		if c.isSameDomain(link) && !c.shouldExclude(link) {
			filteredLinks = append(filteredLinks, link)

			if !c.isVisited(link) {
				c.queue.Push(link)
			}
		}
	}

	scrapedData := ScrapedData{
		URL:       targetURL,
		Title:     pageTitle,
		Selectors: selectorResults,
		Links:     filteredLinks,
		Console:   extractedConsole,
	}

	// Write this data to its own file immediately
	if err := c.fileWriter.WriteURLData(scrapedData); err != nil {
		c.queue.HandleError(targetURL, err, "Error writing data")
	}

	// Still add to results for final combined output if needed
	c.addResult(scrapedData)
}

func (c *Crawler) Run() error {
	// Parse domain from start URL
	startURL, err := url.Parse(c.config.StartURL)
	if err != nil {
		return fmt.Errorf("invalid start URL: %v", err)
	}

	c.config.Domain = startURL.Hostname()

	if c.config.Verbose {
		fmt.Printf("Starting crawl from %s (domain: %s)\n", c.config.StartURL, c.config.Domain)
		fmt.Printf("Using selectors: %v\n", c.config.Selectors)
		fmt.Printf("JavaScript execution: %v\n", c.config.ExecuteJS)
		if c.config.WaitTime > 0 {
			fmt.Printf("Wait time after page load: %v\n", c.config.WaitTime)
		}
		if len(c.config.AttributeNames) > 0 {
			fmt.Printf("Extracting attributes: %v\n", c.config.AttributeNames)
		}
		if len(c.config.ExcludePath) > 0 {
			fmt.Printf("Excluding paths containing: %v\n", c.config.ExcludePath)
		}
	}

	// Start with the first URL
	c.queue.Push(c.config.StartURL)

	// Process queue until empty or max depth reached
	depth := 0
	for !c.queue.IsEmpty() && depth < c.config.MaxDepth {
		var levelURLs []string

		// Get all URLs at the current depth
		for !c.queue.IsEmpty() {
			url, ok := c.queue.Pop()
			if !ok {
				break
			}
			levelURLs = append(levelURLs, url)
		}

		if c.config.Verbose {
			fmt.Printf("Processing depth %d with %d URLs\n", depth, len(levelURLs))
		}

		// Process all URLs at this depth with limited concurrency
		semaphore := make(chan struct{}, c.config.Concurrency)

		for _, url := range levelURLs {
			c.wg.Add(1)
			semaphore <- struct{}{}

			go func(url string) {
				c.scrape(url)
				<-semaphore
			}(url)
		}

		// Wait for all URLs at this depth to finish
		c.wg.Wait()
		depth++
	}

	// Write results to JSON file
	file, err := os.Create(c.config.OutputFile)
	if err != nil {
		return fmt.Errorf("error creating output file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(c.results); err != nil {
		return fmt.Errorf("error encoding results to JSON: %v", err)
	}

	if c.config.Verbose {
		fmt.Printf("Crawl completed. Visited %d pages. Results saved to %s\n", len(c.visited), c.config.OutputFile)
	}

	return nil
}

func main() {
	// Parse command line flags
	startURL := flag.String("url", "", "Starting URL to crawl (required)")
	outputFile := flag.String("output", "scrape_results.json", "Output JSON file name")
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	maxDepth := flag.Int("depth", 5, "Maximum crawl depth")
	concurrency := flag.Int("concurrency", 3, "Maximum number of concurrent requests")
	waitTime := flag.Duration("wait", 2*time.Second, "Time to wait after page load for JavaScript execution")
	timeout := flag.Duration("timeout", *waitTime+60*time.Second, "HTTP request timeout")
	selectorsFlag := flag.String("selectors", "h1,h2", "CSS selectors separated by comma")
	attributesFlag := flag.String("attributes", "", "Attributes to extract (comma separated). If empty, extracts common attributes")
	excludePathFlag := flag.String("exclude", "", "Exclude URLs containing these paths (comma separated)")
	executeJS := flag.Bool("execute-js", false, "Execute JavaScript on the page")
	script := flag.String("script", "", "Execute script JavaScript on the page")

	flag.Parse()

	if *startURL == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Parse selectors and exclude paths
	selectors := strings.Split(*selectorsFlag, ",")
	for i := range selectors {
		selectors[i] = strings.TrimSpace(selectors[i])
	}

	// Parse attributes to extract
	var attributeNames []string
	if *attributesFlag != "" {
		attributeNames = strings.Split(*attributesFlag, ",")
		for i := range attributeNames {
			attributeNames[i] = strings.TrimSpace(attributeNames[i])
		}
	}

	// Parse exclude paths
	var excludePaths []string
	if *excludePathFlag != "" {
		excludePaths = strings.Split(*excludePathFlag, ",")
		for i := range excludePaths {
			excludePaths[i] = strings.TrimSpace(excludePaths[i])
		}
	}

	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(*outputFile)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Fatalf("Error creating output directory: %v", err)
		}
	}

	// Configure and run the crawler
	config := Configuration{
		StartURL:       *startURL,
		OutputFile:     *outputFile,
		Verbose:        *verbose,
		Selectors:      selectors,
		AttributeNames: attributeNames,
		ExcludePath:    excludePaths,
		MaxDepth:       *maxDepth,
		Concurrency:    *concurrency,
		Timeout:        *timeout,
		ExecuteJS:      *executeJS,
		WaitTime:       *waitTime,
		Script:         *script,
	}

	crawler, err := NewCrawler(config)
	if err != nil {
		log.Fatalf("Failed to initialize crawler: %v", err)
	}
	defer crawler.Close() // Ensure browser resources are cleaned up

	if err := crawler.Run(); err != nil {
		log.Fatalf("Crawler error: %v", err)
	}
}
