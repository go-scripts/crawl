# Go Web Crawler

A high-performance web crawler written in Go that supports AI integration for processing crawled data.

## Table of Contents
- [Installation](#installation)
- [Basic Usage](#basic-usage)
- [AI Integration](#ai-integration)
  - [Overview](#overview)
  - [Configuration Options](#configuration-options)
  - [Example Usage](#example-usage)
  - [Error Codes and Troubleshooting](#error-codes-and-troubleshooting)
  - [API Requirements](#api-requirements)
  - [Performance Considerations](#performance-considerations)
- [Contributing](#contributing)
- [License](#license)

## Installation

```bash
git clone https://github.com/yourusername/web-crawler.git
cd web-crawler
go build
```

## Basic Usage

```bash
./crawl -url https://example.com -depth 2 -output results.json
```

## AI Integration

### Overview

The web crawler includes AI integration capabilities that allow you to process crawled data through an AI model. This feature enables you to:

- Generate summaries of crawled pages
- Extract key insights from crawled content
- Categorize and analyze web content automatically
- Transform raw crawl results into structured data
- Generate reports based on crawled information

The AI processing happens after each page is crawled, with results being written to a specified output location. The system uses a template-based approach for generating queries to the AI model, providing flexibility in how you interact with the underlying API.

### Configuration Options

The following configuration options are available for AI integration:

| Option | Flag | Default Value | Description |
|--------|------|---------------|-------------|
| Enable AI | `--ai` | `false` | Enable AI processing of crawled data |
| API Endpoint | `--ai-endpoint` | `http://localhost:8080/v1/chat/completions` | URL of the AI API |
| System Prompt | `--ai-system-prompt` | `You are a helpful assistant that analyzes web content.` | System prompt for AI |
| Output Path | `--ai-output` | `` | Path for AI-generated output |
| Query Template | `--ai-query-template` | `<JSON_RESULT>` | Template for AI queries |
| Temperature | `--ai-temp` | `0.7` | Temperature setting for AI responses |
| Reasoning Effort | `--ai-reasoning` | `auto` | Reasoning effort (none, low, medium, high, auto) |
| Context Size | `--ai-context` | `4096` | Maximum context size for AI |

To configure the AI integration, use command-line flags:

```bash
./crawl --url https://example.com --depth 2 --ai --ai-output=ai_analysis.json --ai-endpoint=https://your-ai-api.com/api
```

### Example Usage

**Basic AI Processing**

Process a website and generate AI insights:

```bash
./crawl --url https://example.com --depth 3 --ai --ai-output=ai_results.json
```

**Custom Query Template**

Use a custom template to extract specific information:

```bash
./crawl --url https://example.com --ai --ai-query-template="Extract all product information from this data: {{ JSON_RESULT }}" --ai-output=products.json
```

**Batch Processing with Different Parameters**

```bash
#!/bin/bash
urls=("https://site1.com" "https://site2.com" "https://site3.com")
for url in "${urls[@]}"; do
  domain=$(echo $url | awk -F/ '{print $3}')
  ./crawl --url $url --depth 2 --ai --ai-output="results_${domain}.json" --ai-reasoning=high
done
```

**Integration with Other Tools**

```bash
./crawl --url https://example.com --ai --ai-output=data.json && python analyze_results.py data.json
```

### Error Codes and Troubleshooting

| Error Code | Description | Resolution |
|------------|-------------|------------|
| `AI001` | Missing API endpoint | Specify the API endpoint with `--ai-endpoint` |
| `AI002` | Missing output path | Provide an output path with `--ai-output` |
| `AI003` | Invalid query template | Ensure template contains `{{ JSON_RESULT }}` |
| `AI004` | Context size out of range | Use a context size between 1 and 32768 |
| `AI005` | Invalid reasoning effort | Use one of: auto, none, low, medium, high |
| `AI006` | API request failed | Check API endpoint and internet connection |
| `AI007` | API rate limit exceeded | Reduce crawl speed or implement delays |
| `AI008` | Template parsing error | Check the query template format |
| `AI009` | Output file write error | Check file permissions and disk space |

**Common Issues and Solutions:**

1. **API Connection Failures**
   
   ```
   ERROR: Failed to connect to AI API: connection refused
   ```
   
   Ensure the API endpoint is correct and the service is running. Check firewall settings.

2. **Rate Limiting**
   
   ```
   WARNING: Rate limit reached, backing off for 5s
   ```
   
   The crawler includes exponential backoff. If you see this frequently, consider reducing concurrency or increasing rate limits.

3. **File Permission Issues**
   
   ```
   ERROR: Cannot write to output file: permission denied
   ```
   
   Check that the specified output directory exists and has write permissions.

### API Requirements

The AI integration is designed to work with OpenAI-compatible API endpoints, including:

1. **OpenAI API**
   - Requires an API key with appropriate permissions
   - Supports models like GPT-3.5 and GPT-4

2. **Local LLM Servers**
   - Compatible with llama.cpp server
   - Works with Ollama
   - Compatible with any OpenAI-compatible API

**Setting Up a Local API Server:**

```bash
# Example using llama.cpp server
git clone https://github.com/ggerganov/llama.cpp
cd llama.cpp
make
./server -m models/llama-7b.gguf -c 2048 --port 8080
```

Then run the crawler with:

```bash
./crawl --url https://example.com --ai --ai-endpoint=http://localhost:8080/v1/chat/completions
```

### Performance Considerations

The AI processing can significantly impact the overall performance of the crawler:

1. **Memory Usage**
   - AI processing increases memory usage, especially with large context sizes
   - For large crawls, consider reducing the context size (`--ai-context`)

2. **Processing Time**
   - AI inference adds processing time per page
   - Benchmark results show approximately 1-3 seconds of additional processing time per page with a local LLM

3. **Rate Limiting**
   - Most API providers have rate limits
   - The crawler implements exponential backoff and rate limiting
   - Consider adjusting the concurrency settings for optimal throughput

4. **Optimizing Performance**
   - Use shorter context sizes for faster processing
   - Choose lower reasoning effort for less complex tasks
   - Consider batching requests for efficiency

**Benchmark Comparison:**

| Configuration | Pages/min (no AI) | Pages/min (with AI) |
|---------------|-------------------|---------------------|
| Single thread | ~60 | ~20 |
| 5 threads | ~250 | ~80 |
| 10 threads | ~400 | ~120 |

These figures vary based on website complexity, network conditions, and the specific AI model in use.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

