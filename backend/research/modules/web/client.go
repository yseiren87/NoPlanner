package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	intelligencev1 "noplanner/backend/proto/dist/golang/intelligence/v1"
	"noplanner/backend/research/domains/investigation"
)

type Client struct {
	intelligence intelligencev1.IntelligenceServiceClient
	httpClient   *http.Client
	allowPrivate bool
}

func New(connection grpc.ClientConnInterface) *Client {
	var intelligence intelligencev1.IntelligenceServiceClient
	if connection != nil {
		intelligence = intelligencev1.NewIntelligenceServiceClient(connection)
	}
	return &Client{intelligence: intelligence, httpClient: &http.Client{Timeout: 30 * time.Second}}
}

func NewWithClient(intelligence intelligencev1.IntelligenceServiceClient, httpClient *http.Client) *Client {
	return &Client{intelligence: intelligence, httpClient: httpClient, allowPrivate: true}
}

func (c *Client) Search(query string) ([]investigation.SearchHit, error) {
	if c.intelligence == nil {
		return nil, ErrNotConfigured
	}
	response, err := c.intelligence.SearchWeb(context.Background(), &intelligencev1.SearchWebRequest{Query: query})
	if status.Code(err) == codes.FailedPrecondition {
		return nil, ErrNotConfigured
	}
	if err != nil {
		return nil, err
	}
	hits := make([]investigation.SearchHit, 0, len(response.GetSources()))
	for _, source := range response.GetSources() {
		hits = append(hits, investigation.SearchHit{Title: source.GetTitle(), URL: source.GetUrl(), Snippet: source.GetSnippet(), Publisher: source.GetPublisher()})
	}
	return hits, nil
}

var (
	scriptPattern = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)
	tagPattern    = regexp.MustCompile(`(?s)<[^>]+>`)
	titlePattern  = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	spacePattern  = regexp.MustCompile(`\s+`)
)

func (c *Client) Fetch(rawURL string) (investigation.Page, error) {
	if !c.allowPrivate {
		if err := validatePublicURL(rawURL); err != nil {
			return investigation.Page{}, err
		}
	}
	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return investigation.Page{}, err
	}
	request.Header.Set("User-Agent", "NoPlannerResearch/1.0")
	client := *c.httpClient
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if !c.allowPrivate {
			if err := validatePublicURL(request.URL.String()); err != nil {
				return err
			}
		}
		if previousRedirect != nil {
			return previousRedirect(request, via)
		}
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		return investigation.Page{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return investigation.Page{}, fmt.Errorf("source status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return investigation.Page{}, err
	}
	raw := string(body)
	title := ""
	if match := titlePattern.FindStringSubmatch(raw); len(match) > 1 {
		title = strings.TrimSpace(html.UnescapeString(tagPattern.ReplaceAllString(match[1], " ")))
	}
	plain := spacePattern.ReplaceAllString(html.UnescapeString(tagPattern.ReplaceAllString(scriptPattern.ReplaceAllString(raw, " "), " ")), " ")
	if len(plain) > 12000 {
		plain = plain[:12000]
	}
	hash := sha256.Sum256(body)
	return investigation.Page{URL: response.Request.URL.String(), Title: title, Text: strings.TrimSpace(plain), SourceLocation: "response body", ContentHash: hex.EncodeToString(hash[:]), ContentType: response.Header.Get("Content-Type")}, nil
}

func validatePublicURL(rawURL string) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return errors.New("invalid source URL")
	}
	addresses, err := net.LookupIP(parsed.Hostname())
	if err != nil {
		return fmt.Errorf("resolve source host: %w", err)
	}
	for _, address := range addresses {
		if address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsUnspecified() {
			return errors.New("source URL resolves to a non-public address")
		}
	}
	return nil
}

var ErrNotConfigured = fmt.Errorf("gemini google search grounding is not configured")
