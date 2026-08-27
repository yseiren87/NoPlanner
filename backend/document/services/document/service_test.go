package document

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	documentdomain "noplanner/backend/document/domains/document"
	"noplanner/backend/document/modules/llm"
	"noplanner/backend/document/modules/webfetch"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	documentv1 "noplanner/backend/proto/dist/golang/document/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type memoryStore struct {
	values       map[string][]documentdomain.Version
	elements     []documentdomain.Element
	requirements []documentdomain.Requirement
	dependencies []documentdomain.Dependency
}

func TestVersionMessageReportsPartialParseIssues(t *testing.T) {
	message := versionMessage(documentdomain.Version{Extraction: documentdomain.Extraction{
		Issues: []documentdomain.ParseIssue{{Scope: "page:2", Message: "OCR required"}},
	}}, nil)
	if message.GetParseStatus() != documentv1.ParseStatus_PARSE_STATUS_PARTIAL || len(message.GetParseIssues()) != 1 || message.GetParseIssues()[0].GetScope() != "page:2" {
		t.Fatalf("partial parse response = %#v", message)
	}

	complete := versionMessage(documentdomain.Version{}, nil)
	if complete.GetParseStatus() != documentv1.ParseStatus_PARSE_STATUS_COMPLETE || len(complete.GetParseIssues()) != 0 {
		t.Fatalf("complete parse response = %#v", complete)
	}
}

func (s *memoryStore) ReplaceElements(_ context.Context, _ string, _ uint32, _ string, elements []documentdomain.Element) error {
	s.elements = append([]documentdomain.Element(nil), elements...)
	return nil
}
func (s *memoryStore) ReplaceRequirements(_ context.Context, _ string, _ uint32, _ string, r []documentdomain.Requirement, d []documentdomain.Dependency) error {
	s.requirements = append([]documentdomain.Requirement(nil), r...)
	s.dependencies = append([]documentdomain.Dependency(nil), d...)
	return nil
}

func newMemoryStore() *memoryStore {
	return &memoryStore{values: map[string][]documentdomain.Version{}}
}

func (s *memoryStore) Create(_ context.Context, projectID string, input documentdomain.VersionInput) (documentdomain.Version, error) {
	value := documentdomain.Version{
		DocumentID: "document-1", ProjectID: projectID, Number: 1, FileName: input.FileName,
		MediaType: input.MediaType, SizeBytes: input.SizeBytes, SHA256: input.SHA256, ObjectKey: input.ObjectKey, CreatedAt: time.Now(), Extraction: input.Extraction,
		SourceURL: input.SourceURL,
	}
	s.values[value.DocumentID] = []documentdomain.Version{value}
	return value, nil
}

func (s *memoryStore) AddVersion(_ context.Context, documentID string, input documentdomain.VersionInput) (documentdomain.Version, error) {
	versions, ok := s.values[documentID]
	if !ok {
		return documentdomain.Version{}, documentdomain.ErrNotFound
	}
	value := documentdomain.Version{
		DocumentID: documentID, ProjectID: versions[0].ProjectID, Number: uint32(len(versions) + 1), FileName: input.FileName,
		MediaType: input.MediaType, SizeBytes: input.SizeBytes, SHA256: input.SHA256, ObjectKey: input.ObjectKey, CreatedAt: time.Now(), Extraction: input.Extraction,
		SourceURL: input.SourceURL,
	}
	s.values[documentID] = append(versions, value)
	return value, nil
}

func (s *memoryStore) GetVersion(_ context.Context, documentID string, number uint32) (documentdomain.Version, error) {
	versions := s.values[documentID]
	if number == 0 || int(number) > len(versions) {
		return documentdomain.Version{}, documentdomain.ErrNotFound
	}
	return versions[number-1], nil
}

func (s *memoryStore) ListVersions(_ context.Context, documentID string) ([]documentdomain.Version, error) {
	values, ok := s.values[documentID]
	if !ok {
		return nil, documentdomain.ErrNotFound
	}
	return append([]documentdomain.Version(nil), values...), nil
}

type memoryBlobs struct {
	values map[string][]byte
	broken bool
}

type fakeWeb struct {
	page webfetch.Page
	err  error
}

type fakeAnalyzer struct {
	elements []llm.Element
	analysis llm.RequirementAnalysis
	err      error
	prompts  *[]string
}

func (f fakeAnalyzer) ExtractRequirements(context.Context, string) (llm.RequirementAnalysis, error) {
	return f.analysis, f.err
}

func (f fakeAnalyzer) Extract(_ context.Context, prompt string) ([]llm.Element, error) {
	if f.prompts != nil {
		*f.prompts = append(*f.prompts, prompt)
	}
	return f.elements, f.err
}
func (f fakeAnalyzer) Model() string { return "test-model" }

func (f fakeWeb) Fetch(context.Context, string) (webfetch.Page, error) {
	return f.page, f.err
}

func (s *memoryBlobs) Put(_ context.Context, key, _ string, content []byte) error {
	if _, exists := s.values[key]; !exists {
		s.values[key] = append([]byte(nil), content...)
	}
	return nil
}

func (s *memoryBlobs) Get(_ context.Context, key string) ([]byte, error) {
	value, ok := s.values[key]
	if !ok {
		return nil, errors.New("blob not found")
	}
	if s.broken {
		return []byte("corrupted"), nil
	}
	return append([]byte(nil), value...), nil
}

func TestDocumentVersionsPreservePreviousOriginals(t *testing.T) {
	store := newMemoryStore()
	blobs := &memoryBlobs{values: map[string][]byte{}}
	service := New(store, blobs, nil, nil, "document", "test")

	first, err := service.CreateDocument(context.Background(), &documentv1.CreateDocumentRequest{
		ProjectId: "project-1", FileName: "plan.md", MediaType: "text/markdown", Original: []byte("first plan"),
	})
	if err != nil || first.Version != 1 || first.Sha256 == "" {
		t.Fatalf("CreateDocument() = %#v, %v", first, err)
	}
	if len(first.Blocks) != 1 || first.Blocks[0].Location.GetParagraphNumber() != 1 {
		t.Fatalf("stored location = %#v", first.Blocks)
	}
	second, err := service.AddDocumentVersion(context.Background(), &documentv1.AddDocumentVersionRequest{
		DocumentId: first.DocumentId, FileName: "plan.md", MediaType: "text/markdown", Original: []byte("revised plan"),
	})
	if err != nil || second.Version != 2 || second.Sha256 == first.Sha256 {
		t.Fatalf("AddDocumentVersion() = %#v, %v", second, err)
	}
	previous, err := service.GetDocumentVersion(context.Background(), &documentv1.GetDocumentVersionRequest{DocumentId: first.DocumentId, Version: 1})
	if err != nil || string(previous.Original) != "first plan" || previous.Sha256 != first.Sha256 {
		t.Fatalf("previous version = %#v, %v", previous, err)
	}
	latest, err := service.GetDocumentVersion(context.Background(), &documentv1.GetDocumentVersionRequest{DocumentId: first.DocumentId, Version: 2})
	if err != nil || string(latest.Original) != "revised plan" {
		t.Fatalf("latest version = %#v, %v", latest, err)
	}
	listed, err := service.ListDocumentVersions(context.Background(), &documentv1.ListDocumentVersionsRequest{DocumentId: first.DocumentId})
	if err != nil || len(listed.Versions) != 2 || len(listed.Versions[0].Original) != 0 {
		t.Fatalf("ListDocumentVersions() = %#v, %v", listed, err)
	}
}

func TestDocumentVersionRejectsCorruptedOriginal(t *testing.T) {
	store := newMemoryStore()
	blobs := &memoryBlobs{values: map[string][]byte{}}
	service := New(store, blobs, nil, nil, "document", "test")
	created, err := service.CreateDocument(context.Background(), &documentv1.CreateDocumentRequest{
		ProjectId: "project-1", FileName: "plan.txt", MediaType: "text/plain", Original: []byte("original"),
	})
	if err != nil {
		t.Fatal(err)
	}
	blobs.broken = true
	_, err = service.GetDocumentVersion(context.Background(), &documentv1.GetDocumentVersionRequest{DocumentId: created.DocumentId, Version: 1})
	if status.Code(err) != codes.Internal {
		t.Fatalf("corrupt original code = %s", status.Code(err))
	}
}

func TestDocumentRejectsEmptyOriginal(t *testing.T) {
	service := New(newMemoryStore(), &memoryBlobs{values: map[string][]byte{}}, nil, nil, "document", "test")
	_, err := service.CreateDocument(context.Background(), &documentv1.CreateDocumentRequest{ProjectId: "project-1", FileName: "empty.txt", MediaType: "text/plain"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty original code = %s", status.Code(err))
	}
}

func TestCreateDocumentFromURLStoresFetchedHTML(t *testing.T) {
	store := newMemoryStore()
	blobs := &memoryBlobs{values: map[string][]byte{}}
	web := fakeWeb{page: webfetch.Page{
		URL: "https://example.com/plan", FileName: "example.com.html", MediaType: "text/html",
		Content: []byte("<html><body><h1>Purpose</h1><p>Validate facts.</p></body></html>"),
	}}
	service := New(store, blobs, web, nil, "document", "test")
	created, err := service.CreateDocumentFromUrl(context.Background(), &documentv1.CreateDocumentFromUrlRequest{ProjectId: "project-1", Url: web.page.URL})
	if err != nil || created.FileName != "example.com.html" || created.SourceUrl != web.page.URL || created.ExtractedText != "Purpose\n\nValidate facts." || len(created.Blocks) != 2 {
		t.Fatalf("CreateDocumentFromUrl() = %#v, %v", created, err)
	}
}

func TestAnalyzeDocumentVersionLinksElementsToSourceLocations(t *testing.T) {
	store := newMemoryStore()
	blobs := &memoryBlobs{values: map[string][]byte{}}
	analyzer := fakeAnalyzer{elements: []llm.Element{
		{Type: "purpose", Content: "현실적인 기획을 검증한다.", Confidence: 0.96, SourceBlockOrdinal: 1},
		{Type: "assumption", Content: "필요한 데이터가 존재한다.", Confidence: 0.72, SourceBlockOrdinal: 2},
	}}
	service := New(store, blobs, nil, analyzer, "document", "test")
	created, err := service.CreateDocument(context.Background(), &documentv1.CreateDocumentRequest{ProjectId: "project-1", FileName: "plan.md", MediaType: "text/markdown", Original: []byte("# 목적\n\n현실적인 기획을 검증한다.\n\n필요한 데이터가 존재한다.")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.AnalyzeDocumentVersion(context.Background(), &documentv1.AnalyzeDocumentVersionRequest{DocumentId: created.DocumentId, Version: 1, OutputLanguage: commonv1.OutputLanguage_OUTPUT_LANGUAGE_KOREAN})
	if err != nil || len(result.Elements) != 2 || result.Model != "test-model" {
		t.Fatalf("AnalyzeDocumentVersion() = %#v, %v", result, err)
	}
	if result.Elements[1].SourceLocation.GetParagraphNumber() != 1 || result.Elements[1].SourceLocation.GetSectionPath()[0] != "목적" || len(store.elements) != 2 {
		t.Fatalf("source mapping = %#v", result.Elements[1])
	}
}

func TestAnalyzeDocumentVersionRejectsInventedBlockReference(t *testing.T) {
	store := newMemoryStore()
	blobs := &memoryBlobs{values: map[string][]byte{}}
	service := New(store, blobs, nil, fakeAnalyzer{elements: []llm.Element{{Type: "claim", Content: "invented", Confidence: .9, SourceBlockOrdinal: 99}}}, "document", "test")
	created, err := service.CreateDocument(context.Background(), &documentv1.CreateDocumentRequest{ProjectId: "project-1", FileName: "plan.txt", MediaType: "text/plain", Original: []byte("source")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.AnalyzeDocumentVersion(context.Background(), &documentv1.AnalyzeDocumentVersionRequest{DocumentId: created.DocumentId, Version: 1, OutputLanguage: commonv1.OutputLanguage_OUTPUT_LANGUAGE_KOREAN})
	if status.Code(err) != codes.Unavailable || len(store.elements) != 0 {
		t.Fatalf("invalid reference = %s, %#v", status.Code(err), store.elements)
	}
}

func TestAnalyzeRequirementsBuildsValidatedRelationships(t *testing.T) {
	store := newMemoryStore()
	blobs := &memoryBlobs{values: map[string][]byte{}}
	analyzer := fakeAnalyzer{analysis: llm.RequirementAnalysis{Requirements: []llm.Requirement{{Key: "r1", Type: "requirement", Content: "검색할 수 있어야 한다.", Confidence: .94, SourceBlockOrdinal: 2}, {Key: "r2", Type: "policy", Content: "승인된 데이터만 사용한다.", Confidence: .88, SourceBlockOrdinal: 3}}, Dependencies: []llm.Dependency{{Type: "api", Content: "검색 API가 필요하다.", Confidence: .91, SourceBlockOrdinal: 4, RequirementKeys: []string{"r1"}}, {Type: "permission", Content: "데이터 사용 권한이 필요하다.", Confidence: .8, SourceBlockOrdinal: 5, RequirementKeys: []string{"r1", "r2"}}}}}
	service := New(store, blobs, nil, analyzer, "document", "test")
	created, err := service.CreateDocument(context.Background(), &documentv1.CreateDocumentRequest{ProjectId: "p", FileName: "plan.txt", MediaType: "text/plain", Original: []byte("개요\n검색 요구사항\n데이터 정책\n검색 API\n사용 권한")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.AnalyzeRequirements(context.Background(), &documentv1.AnalyzeDocumentVersionRequest{DocumentId: created.DocumentId, Version: 1, OutputLanguage: commonv1.OutputLanguage_OUTPUT_LANGUAGE_KOREAN})
	if err != nil || len(result.Requirements) != 2 || len(result.Dependencies) != 2 {
		t.Fatalf("AnalyzeRequirements=%#v,%v", result, err)
	}
	if result.Dependencies[1].RequirementIds[0] != result.Requirements[0].Id || result.Dependencies[1].RequirementIds[1] != result.Requirements[1].Id || result.Dependencies[0].SourceLocation.GetParagraphNumber() != 4 {
		t.Fatalf("relationships=%#v", result)
	}
}

func TestAnalyzeRequirementsRejectsUnknownRelationship(t *testing.T) {
	store := newMemoryStore()
	blobs := &memoryBlobs{values: map[string][]byte{}}
	analyzer := fakeAnalyzer{analysis: llm.RequirementAnalysis{Dependencies: []llm.Dependency{{Type: "data", Content: "데이터", Confidence: .8, SourceBlockOrdinal: 1, RequirementKeys: []string{"missing"}}}}}
	service := New(store, blobs, nil, analyzer, "document", "test")
	created, err := service.CreateDocument(context.Background(), &documentv1.CreateDocumentRequest{ProjectId: "p", FileName: "plan.txt", MediaType: "text/plain", Original: []byte("데이터")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.AnalyzeRequirements(context.Background(), &documentv1.AnalyzeDocumentVersionRequest{DocumentId: created.DocumentId, Version: 1, OutputLanguage: commonv1.OutputLanguage_OUTPUT_LANGUAGE_KOREAN})
	if status.Code(err) != codes.Unavailable || len(store.dependencies) != 0 {
		t.Fatalf("unknown relationship=%s", status.Code(err))
	}
}

func TestMixedLanguageDocumentKeepsOriginalLocationAndRequestsEnglish(t *testing.T) {
	store := newMemoryStore()
	blobs := &memoryBlobs{values: map[string][]byte{}}
	prompts := []string{}
	analyzer := fakeAnalyzer{prompts: &prompts, elements: []llm.Element{{Type: "purpose", Content: "Validate domestic market data.", Confidence: .9, SourceBlockOrdinal: 2}}}
	service := New(store, blobs, nil, analyzer, "document", "test")
	created, err := service.CreateDocument(context.Background(), &documentv1.CreateDocumentRequest{ProjectId: "p", FileName: "mixed.md", MediaType: "text/markdown", Original: []byte("# 목적 Purpose\n\n국내 market data를 검증한다.")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.AnalyzeDocumentVersion(context.Background(), &documentv1.AnalyzeDocumentVersionRequest{DocumentId: created.DocumentId, Version: 1, OutputLanguage: commonv1.OutputLanguage_OUTPUT_LANGUAGE_ENGLISH})
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 1 || !strings.Contains(prompts[0], "Requested output language: English") || !strings.Contains(prompts[0], "국내 market data") || result.Elements[0].SourceLocation.GetSectionPath()[0] != "목적 Purpose" {
		t.Fatalf("mixed language=%#v,%#v", prompts, result)
	}
}
