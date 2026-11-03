package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrNotConfigured = errors.New("provider is not configured")
var ErrInvalidResponse = errors.New("provider returned an invalid response")

type RequestError struct {
	StatusCode int
}

func (e *RequestError) Error() string {
	return fmt.Sprintf("provider returned status %d", e.StatusCode)
}

type WebSource struct{ Title, URL, Snippet, Publisher string }
type SearchResult struct {
	Sources         []WebSource
	ExecutedQueries []string
}

type Client struct {
	llmProvider, llmModel, llmKey string
	searchKey, searchModel        string
	http                          *http.Client
}

func New(llmProvider, llmModel, llmKey, searchKey, searchModel string) *Client {
	return &Client{llmProvider: llmProvider, llmModel: llmModel, llmKey: llmKey, searchKey: searchKey, searchModel: searchModel, http: &http.Client{Timeout: 90 * time.Second}}
}

func (c *Client) Generate(ctx context.Context, prompt string, jsonResponse bool, maxTokens uint32) (string, error) {
	if c.llmKey == "" || c.llmModel == "" || (c.llmProvider != "claude" && c.llmProvider != "openai" && c.llmProvider != "grok") {
		return "", ErrNotConfigured
	}
	if maxTokens == 0 {
		maxTokens = 4096
	}
	if c.llmProvider == "claude" {
		return c.generateClaude(ctx, prompt, maxTokens)
	}
	return c.generateCompatible(ctx, prompt, jsonResponse)
}

func (c *Client) LLMIdentity() (string, string)    { return c.llmProvider, c.llmModel }
func (c *Client) SearchIdentity() (string, string) { return "google", c.searchModel }

func (c *Client) generateClaude(ctx context.Context, prompt string, maxTokens uint32) (string, error) {
	// Claude Sonnet 5 rejects explicit sampling parameters. Omitting temperature
	// also keeps this request compatible with earlier Claude models.
	payload := map[string]any{"model": c.llmModel, "max_tokens": maxTokens, "messages": []map[string]string{{"role": "user", "content": prompt}}}
	var target struct {
		Content []struct{ Type, Text string } `json:"content"`
	}
	if err := c.call(ctx, "https://api.anthropic.com/v1/messages", payload, &target, map[string]string{"x-api-key": c.llmKey, "anthropic-version": "2023-06-01"}); err != nil {
		return "", err
	}
	for _, item := range target.Content {
		if item.Type == "text" && item.Text != "" {
			return item.Text, nil
		}
	}
	return "", ErrInvalidResponse
}

func (c *Client) generateCompatible(ctx context.Context, prompt string, jsonResponse bool) (string, error) {
	endpoint := "https://api.openai.com/v1/chat/completions"
	if c.llmProvider == "grok" {
		endpoint = "https://api.x.ai/v1/chat/completions"
	}
	payload := map[string]any{"model": c.llmModel, "temperature": 0, "messages": []map[string]string{{"role": "user", "content": prompt}}}
	if jsonResponse {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}
	var target struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := c.call(ctx, endpoint, payload, &target, map[string]string{"Authorization": "Bearer " + c.llmKey}); err != nil {
		return "", err
	}
	if len(target.Choices) != 1 || target.Choices[0].Message.Content == "" {
		return "", ErrInvalidResponse
	}
	return target.Choices[0].Message.Content, nil
}

func (c *Client) Search(ctx context.Context, query string) (SearchResult, error) {
	if c.searchKey == "" || c.searchModel == "" {
		return SearchResult{}, ErrNotConfigured
	}
	endpoint := "https://generativelanguage.googleapis.com/v1beta/models/" + url.PathEscape(c.searchModel) + ":generateContent"
	payload := map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]string{"text": query}}}}, "tools": []any{map[string]any{"google_search": map[string]any{}}}}
	var target struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			GroundingMetadata struct {
				Queries []string `json:"webSearchQueries"`
				Chunks  []struct {
					Web struct{ URI, Title string } `json:"web"`
				} `json:"groundingChunks"`
				Supports []struct {
					Segment struct {
						Text string `json:"text"`
					} `json:"segment"`
					Indices []int `json:"groundingChunkIndices"`
				} `json:"groundingSupports"`
			} `json:"groundingMetadata"`
		} `json:"candidates"`
	}
	if err := c.call(ctx, endpoint, payload, &target, map[string]string{"x-goog-api-key": c.searchKey}); err != nil {
		return SearchResult{}, err
	}
	if len(target.Candidates) == 0 {
		return SearchResult{}, nil
	}
	candidate := target.Candidates[0]
	snippets := map[int][]string{}
	for _, support := range candidate.GroundingMetadata.Supports {
		for _, index := range support.Indices {
			snippets[index] = append(snippets[index], support.Segment.Text)
		}
	}
	fallback := ""
	for _, part := range candidate.Content.Parts {
		fallback += part.Text
	}
	result := SearchResult{ExecutedQueries: candidate.GroundingMetadata.Queries}
	seen := map[string]bool{}
	for index, chunk := range candidate.GroundingMetadata.Chunks {
		if chunk.Web.URI == "" || seen[chunk.Web.URI] {
			continue
		}
		seen[chunk.Web.URI] = true
		snippet := strings.TrimSpace(strings.Join(snippets[index], " "))
		if snippet == "" {
			snippet = strings.TrimSpace(fallback)
		}
		publisher := ""
		if parsed, err := url.Parse(chunk.Web.URI); err == nil {
			publisher = parsed.Hostname()
		}
		result.Sources = append(result.Sources, WebSource{Title: chunk.Web.Title, URL: chunk.Web.URI, Snippet: snippet, Publisher: publisher})
	}
	return result, nil
}

func (c *Client) call(ctx context.Context, endpoint string, payload, target any, headers map[string]string) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("content-type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return &RequestError{StatusCode: response.StatusCode}
	}
	if json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(target) != nil {
		return ErrInvalidResponse
	}
	return nil
}
