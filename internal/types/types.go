package types

// ExtractedContent represents the data extracted from a single webpage
type ExtractedContent struct {
    URL      string
    Title    string
    Content  string
    Links    []string
    Depth    int
}

// ScrapedData represents the complete dataset from a crawling session
type ScrapedData struct {
    StartURL string
    Pages   []ExtractedContent
}

