package webfetch

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestFetchRejectsInternalAndNonHTTPURLs(t *testing.T) {
	fetcher := New()
	for _, rawURL := range []string{
		"http://127.0.0.1/admin",
		"http://[::1]/admin",
		"http://169.254.169.254/latest/meta-data",
		"file:///etc/passwd",
		"https://user:password@example.com/",
	} {
		if _, err := fetcher.Fetch(context.Background(), rawURL); !errors.Is(err, ErrUnsafeURL) {
			t.Fatalf("Fetch(%q) error = %v", rawURL, err)
		}
	}
}

func TestPublicIPClassification(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "169.254.1.1", "100.64.0.1", "::1"} {
		if publicIP(net.ParseIP(value)) {
			t.Fatalf("publicIP(%q) = true", value)
		}
	}
	if !publicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public IP rejected")
	}
}
