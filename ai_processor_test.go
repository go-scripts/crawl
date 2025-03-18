package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
	"time"
)

// Test fixtures
const (
	testSystemPrompt    = "You are a helpful assistant analyzing web crawl data."
	testQueryTemplate   = "System: {{.SystemPrompt}}\n\nAnalyze this JSON data:\n{{ .JSON_RESULT }}"
	testValidJSONData   = `{"url":"https://example.com","title":"Example Domain","content":"This domain is for use in examples."}`
	testInvalidJSONData = `{"url":"https://example.com",INVALID}`
	testAPIResponse     = `{"response":"This is a website about examples and demonstrations. The domain example.com is reserved for illustrative examples in documentation."}`
)

// Mock API server for integration tests
func setupMockAPIServer(t *testing.T, statusCode int, responseBody string, delay time.Duration) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate request method
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Validate request headers
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type: application/json, got %s", contentType)
		}

		// Read and validate request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Error reading request body: %v", err)
		}
		defer r.Body.Close()

		// Check if body is valid JSON
		var requestData map[string]interface{}
		if err := json.Unmarshal(body, &requestData); err != nil {
			t.Errorf("Invalid JSON in request body: %v", err)
		}

		// Simulate processing delay if specified
		if delay > 0 {
			time.Sleep(delay)
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	}))

	t.Cleanup(func() {
		server.Close()
	})

	return server
}

// Setup temp directory for file output tests
func setupTempDir(t *testing.T) string {
	tempDir, err := os.MkdirTemp("", "ai_processor_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	return tempDir
}

// TestGenerateQuery tests the template processing and query generation
func TestGenerateQuery(t *testing.T) {
	testCases := []struct {
		name           string
		systemPrompt   string
		queryTemplate  string
		jsonData       string
		expectedError  bool
		expectedOutput string
	}{
		{
			name:           "Valid JSON with basic template",
			systemPrompt:   testSystemPrompt,
			queryTemplate:  testQueryTemplate,
			jsonData:       testValidJSONData,
			expectedError:  false,
			expectedOutput: "System: You are a helpful assistant analyzing web crawl data.\n\nAnalyze this JSON data:\n" + testValidJSONData,
		},
		{
			name:           "Invalid JSON data",
			systemPrompt:   testSystemPrompt,
			queryTemplate:  testQueryTemplate,
			jsonData:       testInvalidJSONData,
			expectedError:  false, // JSON validity is not checked in generateQuery
			expectedOutput: "System: You are a helpful assistant analyzing web crawl data.\n\nAnalyze this JSON data:\n" + testInvalidJSONData,
		},
		{
			name:           "Invalid template",
			systemPrompt:   testSystemPrompt,
			queryTemplate:  "System: {{.SystemPrompt}}\n\nAnalyze this JSON data:\n{{ .InvalidPlaceholder }}",
			jsonData:       testValidJSONData,
			expectedError:  true,
			expectedOutput: "",
		},
		{
			name:           "Empty JSON data",
			systemPrompt:   testSystemPrompt,
			queryTemplate:  testQueryTemplate,
			jsonData:       "",
			expectedError:  false,
			expectedOutput: "System: You are a helpful assistant analyzing web crawl data.\n\nAnalyze this JSON data:\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create AI processor with test configuration
			config := &AIConfig{
				SystemPrompt:  tc.systemPrompt,
				QueryTemplate: tc.queryTemplate,
			}
			processor, err := NewAIProcessor(config)
			if err != nil {
				t.Fatalf("Failed to create AIProcessor: %v", err)
			}

			// Test generateQuery
			query, err := processor.generateQuery(tc.jsonData)

			// Check error expectation
			if tc.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			} else if !tc.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check output when no error expected
			if !tc.expectedError && query != tc.expectedOutput {
				t.Errorf("Expected query:\n%s\n\nGot:\n%s", tc.expectedOutput, query)
			}
		})
	}
}

// TestWriteResponse tests the response writing functionality
func TestWriteResponse(t *testing.T) {
	tempDir := setupTempDir(t)

	testCases := []struct {
		name        string
		filename    string
		content     string
		expectError bool
	}{
		{
			name:        "Write to valid file",
			filename:    filepath.Join(tempDir, "valid.txt"),
			content:     "Test content",
			expectError: false,
		},
		{
			name:        "Write to invalid directory",
			filename:    filepath.Join(tempDir, "nonexistent_dir", "invalid.txt"),
			content:     "Test content",
			expectError: true,
		},
		{
			name:        "Write empty content",
			filename:    filepath.Join(tempDir, "empty.txt"),
			content:     "",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create AI processor
			config := &AIConfig{
				OutputPath: tempDir,
			}
			processor, err := NewAIProcessor(config)
			if err != nil {
				t.Fatalf("Failed to create AIProcessor: %v", err)
			}

			// Test writeResponse
			err = processor.writeResponse(tc.filename, tc.content)

			// Check error expectation
			if tc.expectError && err == nil {
				t.Errorf("Expected error but got none")
			} else if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Verify file contents when no error expected
			if !tc.expectError {
				content, err := os.ReadFile(tc.filename)
				if err != nil {
					t.Errorf("Failed to read output file: %v", err)
				} else if string(content) != tc.content {
					t.Errorf("Expected file content: %q, got: %q", tc.content, string(content))
				}
			}
		})
	}
}

// TestProcessJSONResult tests the full processing workflow
func TestProcessJSONResult(t *testing.T) {
	// Setup mock server
	server := setupMockAPIServer(t, http.StatusOK, testAPIResponse, 0)
	tempDir := setupTempDir(t)

	// Test data
	testData := ScrapedData{
		URL:   "https://example.com",
		Title: "Example Domain",
		Selectors: map[string][]ExtractedContent{
			"h1": {
				{
					Text: "This domain is for use in examples.",
					HTML: "<h1>This domain is for use in examples.</h1>",
				},
			},
		},
	}

	// Create test config
	config := &AIConfig{
		Enabled:         true,
		SystemPrompt:    testSystemPrompt,
		QueryTemplate:   testQueryTemplate,
		OutputPath:      tempDir,
		APIEndpoint:     server.URL,
		Temperature:     0.7,
		ReasoningEffort: "auto",
		ContextSize:     4096,
	}

	// Create processor
	processor, err := NewAIProcessor(config)
	if err != nil {
		t.Fatalf("Failed to create AIProcessor: %v", err)
	}

	// Test process
	err = processor.ProcessJSONResult(testData)
	if err != nil {
		t.Fatalf("ProcessJSONResult failed: %v", err)
	}

	// Verify output file exists and contains expected content
	expectedFilename := filepath.Join(tempDir, strings.Replace(testData.URL, "://", "_", 1)+".txt")
	content, err := os.ReadFile(expectedFilename)
	if err != nil {
		t.Errorf("Failed to read output file: %v", err)
	} else if string(content) != strings.Trim(testAPIResponse, "{}\"response:\"") {
		t.Errorf("Expected file content from API response, got: %q", string(content))
	}
}

// TestProcessJSONResultErrors tests error handling in ProcessJSONResult
func TestProcessJSONResultErrors(t *testing.T) {
	testCases := []struct {
		name        string
		serverSetup func(*testing.T) *httptest.Server
		config      *AIConfig
		expectError bool
	}{
		{
			name: "Server error",
			serverSetup: func(t *testing.T) *httptest.Server {
				return setupMockAPIServer(t, http.StatusInternalServerError, `{"error":"Internal server error"}`, 0)
			},
			config: &AIConfig{
				Enabled:         true,
				SystemPrompt:    testSystemPrompt,
				QueryTemplate:   testQueryTemplate,
				Temperature:     0.7,
				ReasoningEffort: "auto",
				ContextSize:     4096,
			},
			expectError: true,
		},
		{
			name: "Timeout error",
			serverSetup: func(t *testing.T) *httptest.Server {
				return setupMockAPIServer(t, http.StatusOK, testAPIResponse, 5*time.Second)
			},
			config: &AIConfig{
				Enabled:         true,
				SystemPrompt:    testSystemPrompt,
				QueryTemplate:   testQueryTemplate,
				Temperature:     0.7,
				ReasoningEffort: "auto",
				ContextSize:     4096,
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock server
			server := tc.serverSetup(t)
			tempDir := setupTempDir(t)

			// Update config
			tc.config.OutputPath = tempDir
			tc.config.APIEndpoint = server.URL

			// Create processor with short timeout for timeout test
			tempClient := &http.Client{
				Timeout: 100 * time.Millisecond,
			}
			processor := &AIProcessor{
				config:        tc.config,
				client:        tempClient,
				queryTemplate: template.Must(template.New("query").Parse(tc.config.QueryTemplate)),
				rateLimiter:   NewRateLimiter(1, 1*time.Second),
			}

			// Test data
			testData := ScrapedData{
				URL:   "https://example.com",
				Title: "Example Domain",
				Selectors: map[string][]ExtractedContent{
					"h1": {
						{
							Text: "This domain is for use in examples.",
							HTML: "<h1>This domain is for use in examples.</h1>",
						},
					},
				},
			}

			// Test process
			err := processor.ProcessJSONResult(testData)

			// Check error expectation
			if tc.expectError && err == nil {
				t.Errorf("Expected error but got none")
			} else if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestAIConfigValidation tests the validation of AIConfig
func TestAIConfigValidation(t *testing.T) {
	testCases := []struct {
		name        string
		config      *AIConfig
		expectError bool
	}{
		{
			name: "Valid configuration",
			config: &AIConfig{
				Enabled:         true,
				SystemPrompt:    testSystemPrompt,
				OutputPath:      "/tmp/output",
				QueryTemplate:   "Template with {{ .JSON_RESULT }}",
				APIEndpoint:     "https://api.example.com",
				Temperature:     0.7,
				ReasoningEffort: "auto",
				ContextSize:     4096,
			},
			expectError: false,
		},
		{
			name: "Missing API endpoint",
			config: &AIConfig{
				Enabled:         true,
				SystemPrompt:    testSystemPrompt,
				OutputPath:      "/tmp/output",
				QueryTemplate:   "Template with {{ .JSON_RESULT }}",
				Temperature:     0.7,
				ReasoningEffort: "auto",
				ContextSize:     4096,
			},
			expectError: true,
		},
		{
			name: "Missing output path",
			config: &AIConfig{
				Enabled:         true,
				SystemPrompt:    testSystemPrompt,
				QueryTemplate:   "Template with {{ .JSON_RESULT }}",
				APIEndpoint:     "https://api.example.com",
				Temperature:     0.7,
				ReasoningEffort: "auto",
				ContextSize:     4096,
			},
			expectError: true,
		},
		{
			name: "Invalid template (missing JSON_RESULT)",
			config: &AIConfig{
				Enabled:         true,
				SystemPrompt:    testSystemPrompt,
				OutputPath:      "/tmp/output",
				QueryTemplate:   "Template without JSON placeholder",
				APIEndpoint:     "https://api.example.com",
				Temperature:     0.7,
				ReasoningEffort: "auto",
				ContextSize:     4096,
			},
			expectError: true,
		},
		{
			name: "Invalid context size (too small)",
			config: &AIConfig{
				Enabled:         true,
				SystemPrompt:    testSystemPrompt,
				OutputPath:      "/tmp/output",
				QueryTemplate:   "Template with {{ .JSON_RESULT }}",
				APIEndpoint:     "https://api.example.com",
				Temperature:     0.7,
				ReasoningEffort: "auto",
				ContextSize:     0,
			},
			expectError: true,
		},
		{
			name: "Invalid context size (too large)",
			config: &AIConfig{
				Enabled:         true,
				SystemPrompt:    testSystemPrompt,
				QueryTemplate:   "Template with {{ .JSON_RESULT }}",
				APIEndpoint:     "https://api.example.com",
				Temperature:     0.7,
				ReasoningEffort: "auto",
				ContextSize:     50000,
			},
			expectError: true,
		},
		{
			name: "Invalid reasoning effort",
			config: &AIConfig{
				Enabled:         true,
				SystemPrompt:    testSystemPrompt,
				OutputPath:      "/tmp/output",
				QueryTemplate:   "Template with {{ .JSON_RESULT }}",
				APIEndpoint:     "https://api.example.com",
				Temperature:     0.7,
				ReasoningEffort: "invalid_setting",
				ContextSize:     4096,
			},
			expectError: true,
		},
	}

	// Run each validation test by creating a crawler with the config
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := Configuration{
				AI: *tc.config,
			}

			_, err := NewCrawler(config)

			if tc.expectError && err == nil {
				t.Errorf("Expected error but got none")
			} else if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestMatchesAnyPattern tests the path matching functionality
func TestMatchesAnyPattern(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		patterns []string
		want     bool
	}{
		{
			name:     "Empty patterns matches everything",
			path:     "/any/path",
			patterns: []string{},
			want:     true,
		},
		{
			name:     "Basic wildcard matching",
			path:     "/api/users.html",
			patterns: []string{"*.html", "*.json"},
			want:     true,
		},
		{
			name:     "Directory wildcard matching",
			path:     "/api/v1/users",
			patterns: []string{"/api/*"},
			want:     true,
		},
		{
			name:     "Multiple patterns with match",
			path:     "/docs/index.md",
			patterns: []string{"*.html", "*.md", "*.json"},
			want:     true,
		},
		{
			name:     "No matching pattern",
			path:     "/api/users.xml",
			patterns: []string{"*.html", "*.json"},
			want:     false,
		},
		{
			name:     "Edge case - root path",
			path:     "/",
			patterns: []string{"/*"},
			want:     true,
		},
		{
			name:     "Edge case - empty path",
			path:     "",
			patterns: []string{"*"},
			want:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesAnyPattern(tc.path, tc.patterns)
			if got != tc.want {
				t.Errorf("matchesAnyPattern(%q, %v) = %v, want %v",
					tc.path, tc.patterns, got, tc.want)
			}
		})
	}
}

// BenchmarkProcessJSONResult benchmarks the AI processing performance
func BenchmarkProcessJSONResult(b *testing.B) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testAPIResponse))
	}))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "benchmark")
	if err != nil {
		b.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test config
	config := &AIConfig{
		Enabled:         true,
		SystemPrompt:    testSystemPrompt,
		QueryTemplate:   testQueryTemplate,
		OutputPath:      tempDir,
		APIEndpoint:     server.URL,
		Temperature:     0.7,
		ReasoningEffort: "auto",
		ContextSize:     4096,
	}

	// Create processor
	processor, err := NewAIProcessor(config)
	if err != nil {
		b.Fatalf("Failed to create AIProcessor: %v", err)
	}

	// Create test data with varying sizes
	testDataSmall := ScrapedData{
		URL:   "https://example.com/small",
		Title: "Small Example",
		Selectors: map[string][]ExtractedContent{
			"p": {
				{
					Text: "This is a small example.",
					HTML: "<p>This is a small example.</p>",
				},
			},
		},
	}

	testDataLarge := ScrapedData{
		URL:       "https://example.com/large",
		Title:     "Large Example",
		Selectors: map[string][]ExtractedContent{},
		Links:     make([]string, 100),
	}

	// Fill links with dummy data
	for i := 0; i < 100; i++ {
		testDataLarge.Links[i] = fmt.Sprintf("https://example.com/page%d", i)
	}

	// Benchmark with small data
	b.Run("SmallData", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err := processor.ProcessJSONResult(testDataSmall)
			if err != nil {
				b.Fatalf("ProcessJSONResult failed: %v", err)
			}
		}
	})

	// Benchmark with large data
	b.Run("LargeData", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err := processor.ProcessJSONResult(testDataLarge)
			if err != nil {
				b.Fatalf("ProcessJSONResult failed: %v", err)
			}
		}
	})
}

// BenchmarkGenerateQuery benchmarks the query generation functionality
func BenchmarkGenerateQuery(b *testing.B) {
	// Create test config and processor
	config := &AIConfig{
		QueryTemplate: testQueryTemplate,
		SystemPrompt:  testSystemPrompt,
	}

	processor, err := NewAIProcessor(config)
	if err != nil {
		b.Fatalf("Failed to create AIProcessor: %v", err)
	}

	// Benchmark with small JSON data
	b.Run("SmallJSON", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := processor.generateQuery(testValidJSONData)
			if err != nil {
				b.Fatalf("generateQuery failed: %v", err)
			}
		}
	})

	// Create larger JSON data
	var largeDataBuilder strings.Builder
	largeDataBuilder.WriteString("{\"items\": [")
	for i := 0; i < 1000; i++ {
		if i > 0 {
			largeDataBuilder.WriteString(",")
		}
		largeDataBuilder.WriteString(fmt.Sprintf("{\"id\": %d, \"value\": \"data value %d\"}", i, i))
	}
	largeDataBuilder.WriteString("]}")
	largeJSONData := largeDataBuilder.String()

	// Benchmark with large JSON data
	b.Run("LargeJSON", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := processor.generateQuery(largeJSONData)
			if err != nil {
				b.Fatalf("generateQuery failed: %v", err)
			}
		}
	})
}
