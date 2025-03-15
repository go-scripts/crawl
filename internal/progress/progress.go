package progress

import (
    "fmt"
    "sync"
    "time"
    
    "github.com/charmbracelet/bubbles/progress"
    tea "github.com/charmbracelet/bubbletea"
)

// ProgressTracker manages multiple progress bars
type ProgressTracker struct {
    overallProgress progress.Model
    totalPages     int
    processedPages int
    mu            sync.Mutex
}

// New creates a new ProgressTracker
func New() *ProgressTracker {
    p := &ProgressTracker{
        overallProgress: progress.New(progress.WithDefaultGradient()),
        processedPages:  0,
    }
    return p
}

// SetTotalPages sets the total number of pages to process
func (p *ProgressTracker) SetTotalPages(total int) {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.totalPages = total
}

// IncrementProcessed increments the number of processed pages
func (p *ProgressTracker) IncrementProcessed() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.processedPages++
    
    // Update progress display
    if p.totalPages > 0 {
        progress := float64(p.processedPages) / float64(p.totalPages)
        fmt.Printf("\rProgress: %s %d/%d pages", 
            p.overallProgress.ViewAs(progress), 
            p.processedPages, 
            p.totalPages)
    }
}

// GetProgress returns the current progress as a percentage
func (p *ProgressTracker) GetProgress() float64 {
    p.mu.Lock()
    defer p.mu.Unlock()
    if p.totalPages == 0 {
        return 0
    }
    return float64(p.processedPages) / float64(p.totalPages)
}

// StartProcessingPage indicates that a page is being processed
func (p *ProgressTracker) StartProcessingPage(url string) {
    fmt.Printf("\nProcessing: %s", url)
}

// FinishProcessingPage indicates that a page has been processed
func (p *ProgressTracker) FinishProcessingPage(url string) {
    p.IncrementProcessed()
}

