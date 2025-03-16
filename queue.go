package main

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/briandowns/spinner"
	"github.com/charmbracelet/log"
)

// Queue for URLs to be processed
type Queue struct {
	items         []string
	mu            sync.Mutex
	spinners      []*spinner.Spinner
	spinnerStatus []bool   // true if in use
	spinnerURLs   []string // URL associated with each spinner
	spinnerMu     sync.Mutex
}

// Push adds a URL to the queue and associates it with an available spinner
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

// Pop removes and returns the next URL from the queue
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

// IsEmpty checks if the queue is empty
func (q *Queue) IsEmpty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) == 0
}

// HandleError processes errors that occur during crawling
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

// UpdateSpinnerMessage updates the message on the spinner associated with a URL
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

