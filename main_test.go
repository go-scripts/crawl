package main

import (
    "encoding/json"
    "testing"
    "time"
    "context"
    
    "github.com/chromedp/chromedp"
    "github.com/stretchr/testify/assert"
)

func TestJavaScriptReturnHandling(t *testing.T) {
    // Setup test configuration
    config := Configuration{
        StartURL:    "about:blank",
        Verbose:     true,
        WaitTime:    1 * time.Second,
        Timeout:     5 * time.Second,
    }
    
    crawler, err := NewCrawler(config)
    assert.NoError(t, err)
    defer crawler.Close()

    tests := []struct {
        name     string
        script   string
        want     string
        wantErr  bool
        wantSize int // Size expectation for large payloads
    }{
        {
            name:   "string return",
            script: "(() => 'test string')()",
            want:   "test string",
        },
        {
            name:   "object return",
            script: "(() => ({ key: 'value', num: 123 }))()",
            want:   `{"key":"value","num":123}`,
        },
        {
            name:   "empty return",
            script: "(() => {})()",
            want:   "null",
        },
        {
            name:   "array return",
            script: "(() => ['a', 'b', 'c'])()",
            want:   `["a","b","c"]`,
        },
        {
            name:   "nested object return",
            script: "(() => ({ a: { b: { c: 'deep' } } }))()",
            want:   `{"a":{"b":{"c":"deep"}}}`,
        },
        {
            name:   "browser native object",
            script: "(() => document.createElement('div'))()",
            want:   `{}`,  // Browser native objects serialize to empty objects
        },
        {
            name:   "large payload return",
            script: `(() => {
                // Generate a large object with many nested properties
                const generateLargeObject = (depth, width, size) => {
                    if (depth === 0 || size <= 0) {
                        return "x".repeat(Math.min(1000, size));
                    }
                    const obj = {};
                    const childSize = Math.floor(size / width);
                    for (let i = 0; i < width && size > 0; i++) {
                        obj['key' + i] = generateLargeObject(depth - 1, width, childSize);
                        size -= childSize;
                    }
                    return obj;
                };
                // Generate object approximately 2MB in size
                return generateLargeObject(3, 4, 2 * 1024 * 1024);
            })()`,
            want: "", // We don't check the exact value, just ensure it processes without error
            wantSize: 2 * 1024 * 1024, // Approximately 2MB
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Create context
            ctx, cancel := context.WithTimeout(crawler.browserCtx, 5*time.Second)
            defer cancel()

            // Execute script
            var result interface{}
            err := chromedp.Run(ctx, chromedp.Evaluate(tt.script, &result))
            
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            
            assert.NoError(t, err)
            
            // Process result
            var returnValue string
            switch v := result.(type) {
            case string:
                returnValue = v
            default:
                jsonBytes, err := json.Marshal(v)
                assert.NoError(t, err)
                returnValue = string(jsonBytes)
            }
            
            if tt.want != "" {
                assert.Equal(t, tt.want, returnValue)
            }
            
            if tt.wantSize > 0 {
                assert.Greater(t, len(returnValue), tt.wantSize/2)
                assert.Less(t, len(returnValue), tt.wantSize*2)
            }
        })
    }
}

