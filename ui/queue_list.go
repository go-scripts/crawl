package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// QueueItem represents a URL in the queue
type QueueItem struct {
	url      string
	status   string
	priority int
}

// FilterValue implements list.Item interface
func (i QueueItem) FilterValue() string { return i.url }

// Title returns the item's title
func (i QueueItem) Title() string { return i.url }

// Description returns the item's description
func (i QueueItem) Description() string {
	return fmt.Sprintf("Priority: %d | Status: %s", i.priority, i.status)
}

// QueueList manages the URL queue with virtualization
type QueueList struct {
	list      list.Model
	width     int
	height    int
	processed int
	total     int
}

// NewQueueList creates a new queue list
func NewQueueList() *QueueList {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(lipgloss.Color("170"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(lipgloss.Color("244"))

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "URL Queue"
	l.Styles.Title = l.Styles.Title.Foreground(lipgloss.Color("240"))

	return &QueueList{
		list:      l,
		processed: 0,
		total:     0,
	}
}

// SetSize updates the list dimensions
func (q *QueueList) SetSize(width, height int) {
	q.width = width
	q.height = height
	q.list.SetSize(width, height)
}

// Update handles UI updates
func (q *QueueList) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	q.list, cmd = q.list.Update(msg)
	return cmd
}

// View renders the component
func (q *QueueList) View() string {
	// Modern bubble tea versions use SetFilteringEnabled and Styles for status messages
	q.list.SetFilteringEnabled(false)
	return q.list.View()
}

// AddURL adds a URL to the queue
func (q *QueueList) AddURL(url string, priority int) {
	item := QueueItem{
		url:      url,
		status:   "pending",
		priority: priority,
	}
	q.list.InsertItem(len(q.list.Items()), item)
	q.total++
	q.updateTitle()
}

// MarkURLProcessed marks a URL as processed
func (q *QueueList) MarkURLProcessed(url string) {
	items := q.list.Items()
	for i, item := range items {
		if qItem, ok := item.(QueueItem); ok && qItem.url == url {
			qItem.status = "processed"
			q.list.SetItem(i, qItem)
			q.processed++
			q.updateTitle()
			break
		}
	}
}

// updateTitle updates the component title with stats
func (q *QueueList) updateTitle() {
	q.list.Title = fmt.Sprintf("URL Queue (%d pending)", q.total-q.processed)
}

// Clear empties the queue
func (q *QueueList) Clear() {
	q.list.SetItems([]list.Item{})
	q.processed = 0
	q.total = 0
	q.updateTitle()
}
