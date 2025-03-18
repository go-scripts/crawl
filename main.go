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
}

type ExtractedScriptContent struct {
	Script string      `json:"script,omitempty"`
	Return interface{} `json:"return,omitempty"`
	Error  error       `json:"-,omitempty"`
}

// ScrapedData represents the structure of the scraped content
type ScrapedData struct {
	URL       string                        `json:"url"`
	Title     string                        `json:"title"`
	Selectors map[string][]ExtractedContent `json:"selectors"`
	Links     []string                      `json:"links"`
	Script    ExtractedScriptContent        `json:"script"`
}

// AIConfig holds settings for AI integration
type AIConfig struct {
	Enabled         bool
	IncludePaths    []string // Path include patterns
	SystemPrompt    string
	OutputPath      string
	QueryTemplate   string
	Temperature     float64
	APIEndpoint     string
	ReasoningEffort string
	ContextSize     int
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
	IncludePath    []string
	MaxDepth       int
	Concurrency    int
	Timeout        time.Duration
	ExecuteJS      bool
	WaitTime       time.Duration
	Script         string
	AI             AIConfig
}

// Queue for URLs to be processed
type Queue struct {
	items []string
	mu    sync.Mutex
}

type FileWriter struct {
	outputDir string
	mu        sync.Mutex
	crawler   *Crawler // Reference to the parent crawler for AI processing
}

func NewFileWriter(outputDir string) (*FileWriter, error) {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Error("error creating output directory", "err", err)
		return nil, err
	}

	return &FileWriter{
		outputDir: outputDir,
		crawler:   nil, // Will be set after crawler initialization
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

	// Process with AI if enabled
	if fw.crawler != nil && fw.crawler.config.AI.Enabled && fw.crawler.aiProcessor != nil {
		if err := fw.crawler.aiProcessor.ProcessJSONResult(data); err != nil {
			log.Error("Failed to process URL data with AI", "url", data.URL, "error", err)
			// Continue execution despite AI processing error
		} else {
			log.Info("Processed URL data with AI", "url", data.URL)
		}
	}

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
	aiProcessor   *AIProcessor
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
	outputDir := filepath.Dir(config.OutputFile)

	// If the output path ends with a slash, it's already a directory
	if strings.HasSuffix(config.OutputFile, "/") {
		outputDir = config.OutputFile
	}

	fileWriter, err := NewFileWriter(outputDir)
	if err != nil {
		return nil, err
	}

	// The FileWriter will need a reference to the crawler for AI processing
	// This will be set after the crawler is created

	// Validate AI configuration if enabled
	if config.AI.Enabled {
		// Check if APIEndpoint and OutputPath are provided
		if config.AI.APIEndpoint == "" {
			return nil, fmt.Errorf("API endpoint is required when AI is enabled")
		}
		if config.AI.OutputPath == "" {
			return nil, fmt.Errorf("output path is required when AI is enabled")
		}

		// Validate QueryTemplate contains the JSON_RESULT placeholder
		if !strings.Contains(config.AI.QueryTemplate, "<JSON_RESULT>") {
			return nil, fmt.Errorf("query template must contain the '<JSON_RESULT>' placeholder")
		}

		// Ensure ContextSize is within a reasonable range
		if config.AI.ContextSize < 1 || config.AI.ContextSize > 32768 {
			return nil, fmt.Errorf("context size must be between 1 and 32768")
		}

		// Validate ReasoningEffort
		validEfforts := map[string]bool{
			"auto":   true,
			"none":   true,
			"low":    true,
			"medium": true,
			"high":   true,
		}
		if !validEfforts[config.AI.ReasoningEffort] {
			return nil, fmt.Errorf("reasoning effort must be one of: auto, none, low, medium, high")
		}
	}

	// Initialize AI processor if enabled
	var aiProcessor *AIProcessor
	if config.AI.Enabled {
		var err error
		aiProcessor, err = NewAIProcessor(&config.AI)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize AI processor: %w", err)
		}
	}

	// Create crawler instance
	crawler := &Crawler{
		config:        config,
		visited:       make(map[string]bool),
		queue:         Queue{},
		browserCtx:    browserCtx,
		browserCancel: browserCancel,
		fileWriter:    fileWriter,
		aiProcessor:   aiProcessor,
	}

	// Set crawler reference in the file writer
	fileWriter.crawler = crawler

	return crawler, nil
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

	// Check includes first - if includes are specified, path must match at least one
	if len(c.config.IncludePath) > 0 {
		included := false
		for _, include := range c.config.IncludePath {
			if strings.Contains(path, include) {
				included = true
				break
			}
		}
		if !included {
			return true // Exclude if not included
		}
	}

	// Then check excludes
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

	var extractedScriptContent ExtractedScriptContent
	if c.config.Script != "" {
		log.Debug("Executing custom script", "url", targetURL)
		var scriptResult interface{}
		err := chromedp.Run(timeoutCtx, chromedp.Evaluate(c.config.Script, &scriptResult))
		if err != nil {
			extractedScriptContent = ExtractedScriptContent{
				Script: c.config.Script,
				Error:  err,
			}
			if c.config.Verbose {
				log.Error("Script execution failed", "url", targetURL, "script", c.config.Script, "error", err)
			}
		} else {
			extractedScriptContent = ExtractedScriptContent{
				Script: c.config.Script,
				Return: scriptResult,
			}

			if c.config.Verbose {
				log.Info("Script execution succeeded", "url", targetURL, "details", extractedScriptContent)
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
		Script:    extractedScriptContent,
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
			"exclusions", c.config.ExcludePath,
			"inclusions", c.config.IncludePath)
	}

	// Start with the first URL
	c.queue.Push(c.config.StartURL)

	// Process queue until empty or max depth reached
	depth := 0
	for !c.queue.IsEmpty() && depth <= c.config.MaxDepth {
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
	outputPath := c.config.OutputFile

	// Check if the output path ends with a slash, which indicates a directory
	if strings.HasSuffix(outputPath, "/") {
		// Ensure the directory exists (should already be created)
		dirPath := outputPath
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			log.Error("error creating output directory", "err", err)
			return err
		}
		// Append a default filename
		outputPath = filepath.Join(dirPath, "output.json")
	}

	file, err := os.Create(outputPath)
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

	// Define variables to hold flag values
	var startURL string
	var outputFile string
	var verbose bool
	var maxDepth int
	var concurrency int
	var waitTime time.Duration
	var timeout time.Duration
	var selectorsFlag string
	var attributesFlag string
	var excludePathFlag string
	var includePathFlag string
	var executeJS bool
	var script string

	// AI-related flags
	var aiEnabled bool
	var aiSystemPrompt string
	var aiOutputPath string
	var aiQueryTemplate string
	var aiTemperature float64
	var aiAPIEndpoint string
	var aiReasoningEffort string
	var aiContextSize int
	// Parse command line flags with both long and short forms
	flag.StringVar(&startURL, "url", "", "Starting URL to crawl (required)")
	flag.StringVar(&startURL, "u", "", "Starting URL to crawl (shorthand)")

	flag.StringVar(&outputFile, "output", "scrape_results.json", "Output JSON file name")
	flag.StringVar(&outputFile, "o", "scrape_results.json", "Output JSON file name (shorthand)")

	flag.BoolVar(&verbose, "verbose", false, "Enable verbose output")
	flag.BoolVar(&verbose, "v", false, "Enable verbose output (shorthand)")

	flag.IntVar(&maxDepth, "depth", 5, "Maximum crawl depth")
	flag.IntVar(&maxDepth, "d", 5, "Maximum crawl depth (shorthand)")

	flag.IntVar(&concurrency, "concurrency", 3, "Maximum number of concurrent requests")
	flag.IntVar(&concurrency, "c", 3, "Maximum number of concurrent requests (shorthand)")

	flag.DurationVar(&waitTime, "wait", 2*time.Second, "Time to wait after page load for JavaScript execution")
	flag.DurationVar(&waitTime, "w", 2*time.Second, "Time to wait after page load for JavaScript execution (shorthand)")

	defaultTimeout := 2*time.Second + 60*time.Second
	flag.DurationVar(&timeout, "timeout", defaultTimeout, "HTTP request timeout")
	flag.DurationVar(&timeout, "t", defaultTimeout, "HTTP request timeout (shorthand)")

	flag.StringVar(&selectorsFlag, "selectors", "", "CSS selectors separated by comma")
	flag.StringVar(&selectorsFlag, "s", "", "CSS selectors separated by comma (shorthand)")

	flag.StringVar(&attributesFlag, "attributes", "", "Attributes to extract (comma separated). If empty, extracts common attributes")
	flag.StringVar(&attributesFlag, "a", "", "Attributes to extract (comma separated). If empty, extracts common attributes (shorthand)")

	flag.StringVar(&excludePathFlag, "exclude", "", "Exclude URLs containing these paths (comma separated)")
	flag.StringVar(&excludePathFlag, "e", "", "Exclude URLs containing these paths (comma separated) (shorthand)")

	flag.StringVar(&includePathFlag, "includes-path", "", "Include only URLs containing these paths (comma separated)")
	flag.StringVar(&includePathFlag, "in", "", "Include only URLs containing these paths (comma separated) (shorthand)")

	flag.BoolVar(&executeJS, "execute-js", false, "Execute JavaScript on the page")
	flag.BoolVar(&executeJS, "j", false, "Execute JavaScript on the page (shorthand)")

	flag.StringVar(&script, "script", "", "JavaScript to execute on the page.")
	flag.StringVar(&script, "x", "", "JavaScript to execute on the page. (shorthand)")

	// AI-related flags
	flag.BoolVar(&aiEnabled, "ai", false, "Enable AI processing of crawl results")
	flag.BoolVar(&aiEnabled, "i", false, "Enable AI processing of crawl results (shorthand)")

	flag.StringVar(&aiSystemPrompt, "system-prompt", "You are an assistant that analyzes web content.", "System prompt for AI processing")
	flag.StringVar(&aiSystemPrompt, "p", "You are an assistant that analyzes web content.", "System prompt for AI processing (shorthand)")

	flag.StringVar(&aiOutputPath, "ai-output", "", "Path to write AI processing results")
	flag.StringVar(&aiOutputPath, "ao", "", "Path to write AI processing results (shorthand)")

	flag.StringVar(&aiQueryTemplate, "query-template", "Analyze this JSON data: <JSON_RESULT>", "Template for AI query with <JSON_RESULT> placeholder")
	flag.StringVar(&aiQueryTemplate, "qt", "Analyze this JSON data: <JSON_RESULT>", "Template for AI query with <JSON_RESULT> placeholder (shorthand)")

	flag.Float64Var(&aiTemperature, "temperature", 0.7, "Temperature setting for AI processing")
	flag.Float64Var(&aiTemperature, "tp", 0.7, "Temperature setting for AI processing (shorthand)")

	flag.StringVar(&aiAPIEndpoint, "api-endpoint", "", "API endpoint for AI processing")
	flag.StringVar(&aiAPIEndpoint, "api", "", "API endpoint for AI processing (shorthand)")

	flag.StringVar(&aiReasoningEffort, "reasoning", "auto", "Reasoning effort setting (auto, none, low, medium, high)")
	flag.StringVar(&aiReasoningEffort, "r", "auto", "Reasoning effort setting (auto, none, low, medium, high) (shorthand)")

	flag.IntVar(&aiContextSize, "context-size", 8192, "Context size for AI processing")
	flag.IntVar(&aiContextSize, "cs", 8192, "Context size for AI processing (shorthand)")
	// Override default Usage function to show both long and short flag forms
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fmt.Fprintf(os.Stderr, "  --url, -u string\n\tStarting URL to crawl (required)\n")
		fmt.Fprintf(os.Stderr, "  --output, -o string\n\tOutput JSON file name (default \"scrape_results.json\")\n")
		fmt.Fprintf(os.Stderr, "  --verbose, -v\n\tEnable verbose output\n")
		fmt.Fprintf(os.Stderr, "  --depth, -d int\n\tMaximum crawl depth (default 5)\n")
		fmt.Fprintf(os.Stderr, "  --concurrency, -c int\n\tMaximum number of concurrent requests (default 3)\n")
		fmt.Fprintf(os.Stderr, "  --wait, -w duration\n\tTime to wait after page load for JavaScript execution (default 2s)\n")
		fmt.Fprintf(os.Stderr, "  --timeout, -t duration\n\tHTTP request timeout (default 62s)\n")
		fmt.Fprintf(os.Stderr, "  --selectors, -s string\n\tCSS selectors separated by comma\n")
		fmt.Fprintf(os.Stderr, "  --attributes, -a string\n\tAttributes to extract (comma separated). If empty, extracts common attributes\n")
	fmt.Fprintf(os.Stderr, "  --exclude, -e string\n\tExclude URLs containing these paths (comma separated)\n")
	fmt.Fprintf(os.Stderr, "  --includes-path, -in string\n\tInclude only URLs containing these paths (comma separated)\n")
		fmt.Fprintf(os.Stderr, "  --execute-js, -j\n\tExecute JavaScript on the page\n")
		fmt.Fprintf(os.Stderr, "  --script, -x string\n\tJavaScript to execute on the page\n")
	fmt.Fprintf(os.Stderr, "  --ai, -i\n\tEnable AI processing of crawl results\n")
		fmt.Fprintf(os.Stderr, "  --system-prompt, -p string\n\tSystem prompt for AI processing (default \"You are an assistant that analyzes web content.\")\n")
		fmt.Fprintf(os.Stderr, "  --ai-output, -ao string\n\tPath to write AI processing results\n")
		fmt.Fprintf(os.Stderr, "  --query-template, -qt string\n\tTemplate for AI query with <JSON_RESULT> placeholder (default \"Analyze this JSON data: <JSON_RESULT>\")\n")
		fmt.Fprintf(os.Stderr, "  --temperature, -tp float\n\tTemperature setting for AI processing (default 0.7)\n")
		fmt.Fprintf(os.Stderr, "  --api-endpoint, -api string\n\tAPI endpoint for AI processing\n")
		fmt.Fprintf(os.Stderr, "  --reasoning, -r string\n\tReasoning effort setting (auto, none, low, medium, high) (default \"auto\")\n")
		fmt.Fprintf(os.Stderr, "  --context-size, -cs int\n\tContext size for AI processing (default 8192)\n")
	}

	flag.Parse()

	if startURL == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Parse selectors and exclude paths
	selectors := []string{}
	if selectorsFlag != "" {
		selectors = strings.Split(selectorsFlag, ",")
		for i := range selectors {
			selectors[i] = strings.TrimSpace(selectors[i])
		}
	}

	// Parse attributes to extract
	var attributeNames []string
	if attributesFlag != "" {
		attributeNames = strings.Split(attributesFlag, ",")
		for i := range attributeNames {
			attributeNames[i] = strings.TrimSpace(attributeNames[i])
		}
	}

	// Parse exclude paths
	var excludePaths []string
	if excludePathFlag != "" {
		excludePaths = strings.Split(excludePathFlag, ",")
		for i := range excludePaths {
			excludePaths[i] = strings.TrimSpace(excludePaths[i])
		}
	}

	// Parse include paths
	var includePaths []string
	if includePathFlag != "" {
		includePaths = strings.Split(includePathFlag, ",")
		for i := range includePaths {
			includePaths[i] = strings.TrimSpace(includePaths[i])
		}
	}
	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(outputFile)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Error("Failed to create output directory", "error", err)
			os.Exit(1)
		}
	}

	// Configure and run the crawler
	config := Configuration{
		StartURL:       startURL,
		OutputFile:     outputFile,
		Verbose:        verbose,
		Selectors:      selectors,
		AttributeNames: attributeNames,
		ExcludePath:    excludePaths,
		IncludePath:    includePaths,
		MaxDepth:       maxDepth,
		Concurrency:    concurrency,
		Timeout:        timeout,
		ExecuteJS:      executeJS,
		WaitTime:       waitTime,
		Script:         script,
		AI: AIConfig{
			Enabled:         aiEnabled,
			SystemPrompt:    aiSystemPrompt,
			OutputPath:      aiOutputPath,
			QueryTemplate:   aiQueryTemplate,
			Temperature:     aiTemperature,
			APIEndpoint:     aiAPIEndpoint,
			ReasoningEffort: aiReasoningEffort,
			ContextSize:     aiContextSize,
		},
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
