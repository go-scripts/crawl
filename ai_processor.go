package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// Message represents a single message in the LlamaRequest
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LlamaRequest represents the request payload for the llama-server
type LlamaRequest struct {
	Messages        []Message `json:"messages"`
	Stream          bool      `json:"stream"`
	Temperature     float64   `json:"temperature"`
	ReasoningEffort string    `json:"reasoning_effort"`
}

// LlamaMessage represents a message in the Llama API response
type LlamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LlamaChoice represents a choice in the Llama API response
type LlamaChoice struct {
	FinishReason string       `json:"finish_reason"`
	Index        int          `json:"index"`
	Message      LlamaMessage `json:"message"`
}

// LlamaResponse represents the response structure from the llama-server
type LlamaResponse struct {
	Choices []LlamaChoice `json:"choices"`
}

// createLlamaRequest creates a new LlamaRequest with the provided system prompt and JSON result
func createLlamaRequest(systemPrompt string, queryTemplate string, jsonResult string) *LlamaRequest {
	userContent := queryTemplate + "\n\n" + jsonResult

	return &LlamaRequest{
		Messages: []Message{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: userContent,
			},
		},
		Stream:          false,
		Temperature:     0.3,
		ReasoningEffort: "medium",
	}
}

// AIProcessor handles processing of crawled data through AI
// RateLimiter implements a token bucket for rate limiting API calls
type RateLimiter struct {
	tokens      int
	maxTokens   int
	refillRate  time.Duration
	lastRefill  time.Time
	tokensMutex sync.Mutex
}

// NewRateLimiter creates a new rate limiter with the specified parameters
func NewRateLimiter(maxTokens int, refillRate time.Duration) *RateLimiter {
	return &RateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// GetToken tries to get a token from the bucket, refilling if necessary
func (r *RateLimiter) GetToken() bool {
	r.tokensMutex.Lock()
	defer r.tokensMutex.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(r.lastRefill)
	tokensToAdd := int(elapsed / r.refillRate)

	if tokensToAdd > 0 {
		r.tokens = min(r.maxTokens, r.tokens+tokensToAdd)
		r.lastRefill = now
	}

	// Check if we have tokens available
	if r.tokens > 0 {
		r.tokens--
		return true
	}

	return false
}

// AIProcessor handles processing of crawled data through AI
type AIProcessor struct {
	client          *http.Client
	config          *AIConfig
	responseWriters map[string]io.Writer
	rateLimiter     *RateLimiter
}

// NewAIProcessor creates a new AIProcessor with the specified configuration
func NewAIProcessor(config *AIConfig) (*AIProcessor, error) {
	if config == nil {
		return nil, errors.New("AI configuration is required")
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 360 * time.Second,
	}

	// Initialize rate limiter with 5 tokens per minute
	rateLimiter := NewRateLimiter(5, 12*time.Second)

	return &AIProcessor{
		client:          client,
		config:          config,
		responseWriters: make(map[string]io.Writer),
		rateLimiter:     rateLimiter,
	}, nil
}

// ProcessJSONResult processes the crawled data through AI with retry logic
func (p *AIProcessor) ProcessJSONResult(data ScrapedData) error {
	// Convert data to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal crawl data to JSON: %w", err)
	}

	// Use JSON data directly
	jsonResult := string(jsonData)

	// Prepare request to AI API
	req, err := p.prepareAIRequest(jsonResult)
	if err != nil {
		return fmt.Errorf("failed to prepare AI request: %w", err)
	}

	// Implement retry logic with exponential backoff
	maxRetries := 5
	baseDelay := 1 * time.Second
	var resp *http.Response

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Apply rate limiting
		if !p.rateLimiter.GetToken() {
			log.Debug("Rate limit exceeded, waiting for token", "url", data.URL)
			time.Sleep(1 * time.Second)
			continue
		}

		// Log the attempt
		if attempt > 0 {
			log.Debug("Retrying AI request", "url", data.URL, "attempt", attempt, "max_retries", maxRetries)
		}

		// Send request to AI API
		var reqErr error
		resp, reqErr = p.client.Do(req)

		// Request successful, break out of the retry loop
		if reqErr == nil && resp.StatusCode == http.StatusOK {
			break
		}

		// If this was the last attempt, return the error
		if attempt == maxRetries {
			if reqErr != nil {
				return fmt.Errorf("failed to send request to AI API after %d attempts: %w", maxRetries, reqErr)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("AI API returned non-OK status code %d after %d attempts: %s",
				resp.StatusCode, maxRetries, string(body))
		}

		// Close response body if request was made but failed
		if reqErr == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Error("AI API returned non-OK status",
				"url", data.URL,
				"status", resp.StatusCode,
				"response", string(body),
				"attempt", attempt)
		} else {
			log.Error("Failed to send request to AI API",
				"url", data.URL,
				"error", reqErr,
				"attempt", attempt)
		}

		// Calculate backoff delay: baseDelay * 2^attempt with jitter
		delay := baseDelay * time.Duration(1<<uint(attempt))
		jitter := time.Duration(rand.Int63n(int64(delay) / 2))
		delay = delay + jitter

		log.Debug("Backing off before retry", "url", data.URL, "delay", delay)
		time.Sleep(delay)

		// Create a new request for the retry
		req, err = p.prepareAIRequest(jsonResult)
		if err != nil {
			return fmt.Errorf("failed to prepare AI request for retry: %w", err)
		}
	}

	// Make sure we have a valid response at this point
	if resp == nil {
		return fmt.Errorf("failed to get a valid response after %d attempts", maxRetries)
	}

	defer resp.Body.Close()

	// Process response
	var llamaResponse LlamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&llamaResponse); err != nil {
		return fmt.Errorf("failed to decode AI API response: %w", err)
	}

	// Check if we have any choices in the response
	if len(llamaResponse.Choices) == 0 {
		return fmt.Errorf("AI API returned empty choices array")
	}

	// Extract the content from the first choice's message
	aiContent := llamaResponse.Choices[0].Message.Content

	log.Info("AI response", "url", data.URL, "content", aiContent, "response", llamaResponse)

	// Determine output filename
	filename := filepath.Join(p.config.OutputPath, sanitizeURLForFilename(data.URL))

	// Write response to file
	if err := p.writeResponse(filename, aiContent); err != nil {
		return fmt.Errorf("failed to write AI response: %w", err)
	}

	log.Debug("Successfully processed with AI", "url", data.URL)
	return nil
}

// writeResponse writes the AI response to the specified file
func (p *AIProcessor) writeResponse(filename string, content string) error {
	// Ensure directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filename, err)
	}
	defer file.Close()

	// Write content
	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("failed to write to file %s: %w", filename, err)
	}

	return nil
}

// prepareAIRequest prepares an HTTP request to the AI API
func (p *AIProcessor) prepareAIRequest(jsonResult string) (*http.Request, error) {
	// Create llama request
	llamaReq := createLlamaRequest(p.config.SystemPrompt, p.config.QueryTemplate, jsonResult)

	// Set temperature from config if available
	if p.config.Temperature > 0 {
		llamaReq.Temperature = p.config.Temperature
	}

	// Set reasoning effort from config if available
	if p.config.ReasoningEffort != "" {
		llamaReq.ReasoningEffort = p.config.ReasoningEffort
	}

	// Marshal request body
	reqBody, err := json.Marshal(llamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", p.config.APIEndpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// Import ScrapedData from main.go instead

// sanitizeURLForFilename converts a URL into a safe, flat filename with .md extension
func sanitizeURLForFilename(url string) string {
	// Replace common URL components that cause issues in filenames
	replacer := strings.NewReplacer(
		"http://", "",
		"https://", "",
		"www.", "",
		"/", "_",
		":", "_",
		"?", "_",
		"&", "_",
		"=", "_",
		"#", "_",
		" ", "_",
		".", "_",
	)

	// Sanitize the URL and ensure it has the .md extension
	sanitized := replacer.Replace(url)

	// Limit filename length to avoid potential issues
	if len(sanitized) > 200 {
		sanitized = sanitized[:200]
	}

	return sanitized + ".md"
}
