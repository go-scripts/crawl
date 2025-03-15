package queue

import (
    "sync"
)

// Queue represents a thread-safe URL queue for crawling
type Queue struct {
    urls       []string
    visited    map[string]bool
    mu        sync.Mutex
}

// New creates a new Queue instance
func New() *Queue {
    return &Queue{
        urls:    make([]string, 0),
        visited: make(map[string]bool),
    }
}

// Add adds a URL to the queue if it hasn't been visited
func (q *Queue) Add(url string) bool {
    q.mu.Lock()
    defer q.mu.Unlock()
    
    if q.visited[url] {
        return false
    }
    
    q.urls = append(q.urls, url)
    return true
}

// Next returns the next URL to process and marks it as visited
func (q *Queue) Next() (string, bool) {
    q.mu.Lock()
    defer q.mu.Unlock()
    
    if len(q.urls) == 0 {
        return "", false
    }
    
    url := q.urls[0]
    q.urls = q.urls[1:]
    q.visited[url] = true
    
    return url, true
}

// IsVisited checks if a URL has been visited
func (q *Queue) IsVisited(url string) bool {
    q.mu.Lock()
    defer q.mu.Unlock()
    return q.visited[url]
}

// Len returns the current length of the queue
func (q *Queue) Len() int {
    q.mu.Lock()
    defer q.mu.Unlock()
    return len(q.urls)
}

// VisitedCount returns the number of visited URLs
func (q *Queue) VisitedCount() int {
    q.mu.Lock()
    defer q.mu.Unlock()
    return len(q.visited)
}

