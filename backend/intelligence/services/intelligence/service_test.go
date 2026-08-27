package intelligence

import (
	"context"
	"testing"

	"noplanner/backend/intelligence/modules/provider"
	intelligencev1 "noplanner/backend/proto/dist/golang/intelligence/v1"
)

type fakeClient struct{}

func (fakeClient) Generate(context.Context, string, bool, uint32) (string, error) {
	return `{"result":"verified"}`, nil
}
func (fakeClient) Search(context.Context, string) (provider.SearchResult, error) {
	return provider.SearchResult{ExecutedQueries: []string{"official source"}, Sources: []provider.WebSource{{Title: "Official", URL: "https://example.test", Snippet: "verified", Publisher: "example.test"}}}, nil
}
func (fakeClient) LLMIdentity() (string, string)    { return "claude", "test-model" }
func (fakeClient) SearchIdentity() (string, string) { return "google", "search-model" }

func TestGenerateAndSearchReturnNormalizedProviderResults(t *testing.T) {
	service := New(fakeClient{}, "intelligence", "test")
	generated, err := service.Generate(context.Background(), &intelligencev1.GenerateRequest{Prompt: "evaluate", JsonResponse: true})
	if err != nil || generated.GetModel() != "test-model" || generated.GetText() == "" {
		t.Fatalf("Generate() = %#v, %v", generated, err)
	}
	searched, err := service.SearchWeb(context.Background(), &intelligencev1.SearchWebRequest{Query: "claim"})
	if err != nil || len(searched.GetSources()) != 1 || searched.GetSources()[0].GetUrl() != "https://example.test" || len(searched.GetExecutedQueries()) != 1 {
		t.Fatalf("SearchWeb() = %#v, %v", searched, err)
	}
}
