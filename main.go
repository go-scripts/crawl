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

	"github.com/charmbracelet/lipgloss"
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
	items []string
	mu    sync.Mutex
}

type FileWriter struct {
	outputDir string
	mu        sync.Mutex
}

func NewFileWriter(outputDir string) (*FileWriter, error) {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Error("error creating output directory", "err", err)
		return nil, err
	}

	return &FileWriter{
		outputDir: outputDir,
	}, nil
}

func (fw *FileWriter) WriteURLData(data ScrapedData) error {
	// Create a safe filename from URL
	u, err := url.Parse(data.URL)
	if err != nil {
		log.Error("Failed to parse URL for file name", "url", data.URL, "error", err)
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
		log.Error("Failed to marshal URL data to JSON", "url", data.URL, "error", err)
		return err
	}

	// Lock to prevent concurrent file access issues
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if err := os.WriteFile(fullPath, jsonData, 0644); err != nil {
		log.Error("Failed to write URL data to file", "url", data.URL, "file", fullPath, "error", err)
		return err
	}

	log.Info("Successfully wrote URL data", "url", data.URL, "file", fullPath)
	return nil
}

func (q *Queue) Push(url string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, url)
}

func (q *Queue) Pop() (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return "", false
	}

	item := q.items[0]
	q.items = q.items[1:]
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

	return &Crawler{
		config:        config,
		visited:       make(map[string]bool),
		queue:         Queue{},
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

	if c.config.Verbose {
		log.Debug("Scraping URL", "url", targetURL)
	}

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
		if c.config.Verbose {
			log.Error("Navigation failed", "url", targetURL, "error", err)
		}
		return
	}

	// Get page title
	if err := chromedp.Run(timeoutCtx, chromedp.Title(&pageTitle)); err != nil {
		if c.config.Verbose {
			log.Error("Failed to get page title", "url", targetURL, "error", err)
		}
		// Continue anyway, title is not critical
	}

	// Get full page HTML
	if err := chromedp.Run(timeoutCtx, chromedp.OuterHTML("html", &pageHTML)); err != nil {
		if c.config.Verbose {
			log.Error("Failed to get page HTML", "url", targetURL, "error", err)
		}
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
			if c.config.Verbose {
				log.Error("Failed to execute selector query", "url", targetURL, "selector", selector, "error", err)
			}
			continue
		}

		// Parse results
		var extractedData []map[string]interface{}
		if err := json.Unmarshal([]byte(nodesJSON), &extractedData); err != nil {
			if c.config.Verbose {
				log.Error("Failed to parse selector results", "url", targetURL, "selector", selector, "error", err)
			}
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
		log.Debug("Executing custom script", "url", targetURL)
		var nodesScriptJSON string
		err := chromedp.Run(timeoutCtx, chromedp.Evaluate(c.config.Script, &nodesScriptJSON))
		if err != nil && c.config.Verbose {
			log.Error("Script execution failed", "url", targetURL, "error", err)
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
		if c.config.Verbose {
			log.Error("Failed to extract links", "url", targetURL, "error", err)
		}
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
		if c.config.Verbose {
			log.Error("Failed to write URL data", "url", targetURL, "error", err)
		}
	}

	// Still add to results for final combined output if needed
	c.addResult(scrapedData)
}

func (c *Crawler) Run() error {
	// Parse domain from start URL
	startURL, err := url.Parse(c.config.StartURL)
	if err != nil {
		log.Error("invalid start URL", "err", err)
		return err
	}

	c.config.Domain = startURL.Hostname()

	if c.config.Verbose {
		log.Info("Starting crawl", "url", c.config.StartURL, "domain", c.config.Domain)
		log.Debug("Configuration details",
			"selectors", c.config.Selectors,
			"js_enabled", c.config.ExecuteJS,
			"wait_time", c.config.WaitTime,
			"attributes", c.config.AttributeNames,
			"exclusions", c.config.ExcludePath)
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
			log.Info("Processing URLs at depth", "depth", depth, "url_count", len(levelURLs))
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
		log.Error("error creating output file", "err", err)
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(c.results); err != nil {
		log.Error("error encoding results to JSON", "err", err)
		return err
	}

	if c.config.Verbose {
		log.Info("Crawl completed successfully",
			"visited_pages", len(c.visited),
			"output_file", c.config.OutputFile)
	}

	return nil
}

func main() {
	// Set log format to JSON
	log.SetFormatter(log.TextFormatter)
	log.SetLevel(log.DebugLevel)
	styles := log.DefaultStyles()
	styles.Levels[log.ErrorLevel] = lipgloss.NewStyle().
		SetString("ERROR!!").
		Padding(0, 1, 0, 1).
		Background(lipgloss.Color("204")).
		Foreground(lipgloss.Color("0"))
	// Add a custom style for key `err`
	styles.Keys["err"] = lipgloss.NewStyle().Foreground(lipgloss.Color("204"))
	styles.Values["err"] = lipgloss.NewStyle().Bold(true)
	log := log.New(os.Stderr)
	log.SetStyles(styles)

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
			log.Error("Failed to create output directory", "error", err)
			os.Exit(1)
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
		log.Error("Failed to initialize crawler", "error", err)
		os.Exit(1)
	}
	defer crawler.Close() // Ensure browser resources are cleaned up

	if err := crawler.Run(); err != nil {
		log.Error("Crawler error", "error", err)
		os.Exit(1)
	}
}
