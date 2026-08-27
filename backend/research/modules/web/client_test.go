package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	intelligencev1 "noplanner/backend/proto/dist/golang/intelligence/v1"
)

type fakeIntelligence struct{ sourceURL string }

func (f fakeIntelligence) GetStatus(context.Context, *commonv1.StatusRequest, ...grpc.CallOption) (*commonv1.StatusResponse, error) {
	return &commonv1.StatusResponse{State: "ready"}, nil
}
func (f fakeIntelligence) Generate(context.Context, *intelligencev1.GenerateRequest, ...grpc.CallOption) (*intelligencev1.GenerateResponse, error) {
	return nil, nil
}
func (f fakeIntelligence) SearchWeb(context.Context, *intelligencev1.SearchWebRequest, ...grpc.CallOption) (*intelligencev1.SearchWebResponse, error) {
	return &intelligencev1.SearchWebResponse{Sources: []*intelligencev1.WebSource{{Title: "Official", Url: f.sourceURL, Snippet: "verified source"}}}, nil
}

func TestSearchAndFetchPreserveOriginalPage(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html><head><title>Official Source</title></head><body><main>verified value 42</main></body></html>")
	}))
	defer server.Close()
	client := NewWithClient(fakeIntelligence{sourceURL: server.URL + "/source"}, server.Client())
	hits, err := client.Search("claim")
	if err != nil || len(hits) != 1 {
		t.Fatalf("Search() = %#v, %v", hits, err)
	}
	if hits[0].Snippet != "verified source" {
		t.Fatalf("snippet = %q", hits[0].Snippet)
	}
	page, err := client.Fetch(hits[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "Official Source" || page.ContentHash == "" || page.SourceLocation == "" {
		t.Fatalf("page = %#v", page)
	}
}

func TestFetchRejectsPrivateAndMetadataAddresses(t *testing.T) {
	client := New(nil)
	for _, rawURL := range []string{"http://127.0.0.1/private", "http://169.254.169.254/latest/meta-data", "http://[::1]/private"} {
		if _, err := client.Fetch(rawURL); err == nil {
			t.Fatalf("private URL was allowed: %s", rawURL)
		}
	}
}
