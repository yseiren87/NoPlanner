package webfetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrUnsafeURL   = errors.New("unsafe web page URL")
	ErrInvalidPage = errors.New("invalid web page response")
)

const maxPageBytes = 10 << 20

var _, carrierGradeNAT, _ = net.ParseCIDR("100.64.0.0/10")

type Page struct {
	URL       string
	FileName  string
	MediaType string
	Content   []byte
}

type Fetcher interface {
	Fetch(context.Context, string) (Page, error)
}

type HTTPFetcher struct {
	client   *http.Client
	resolver *net.Resolver
}

func New() *HTTPFetcher {
	resolver := net.DefaultResolver
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, ErrUnsafeURL
			}
			addresses, err := resolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, candidate := range addresses {
				if publicIP(candidate.IP) {
					return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
				}
			}
			return nil, ErrUnsafeURL
		},
	}
	fetcher := &HTTPFetcher{resolver: resolver}
	fetcher.client = &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return ErrInvalidPage
			}
			return fetcher.validate(request.Context(), request.URL)
		},
	}
	return fetcher
}

func (f *HTTPFetcher) Fetch(ctx context.Context, rawURL string) (Page, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || f.validate(ctx, parsed) != nil {
		return Page{}, ErrUnsafeURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Page{}, ErrUnsafeURL
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("User-Agent", "NoPlanner/0.1 document-fetcher")
	response, err := f.client.Do(request)
	if err != nil {
		return Page{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Page{}, ErrInvalidPage
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || (mediaType != "text/html" && mediaType != "application/xhtml+xml") {
		return Page{}, ErrInvalidPage
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxPageBytes+1))
	if err != nil || len(content) == 0 || len(content) > maxPageBytes {
		return Page{}, ErrInvalidPage
	}
	return Page{URL: response.Request.URL.String(), FileName: safeFileName(response.Request.URL), MediaType: mediaType, Content: content}, nil
}

func (f *HTTPFetcher) validate(ctx context.Context, value *url.URL) error {
	if value == nil || (value.Scheme != "http" && value.Scheme != "https") || value.Hostname() == "" || value.User != nil {
		return ErrUnsafeURL
	}
	addresses, err := f.resolver.LookupIPAddr(ctx, value.Hostname())
	if err != nil || len(addresses) == 0 {
		return ErrUnsafeURL
	}
	for _, address := range addresses {
		if publicIP(address.IP) {
			return nil
		}
	}
	return ErrUnsafeURL
}

func publicIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !carrierGradeNAT.Contains(ip) && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsMulticast()
}

func safeFileName(value *url.URL) string {
	host := strings.NewReplacer(":", "-", "/", "-").Replace(value.Hostname())
	if host == "" {
		host = "web-page"
	}
	return fmt.Sprintf("%s.html", host)
}
