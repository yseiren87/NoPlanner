package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	documentdomain "noplanner/backend/document/domains/document"
	"noplanner/backend/document/modules/blobstore"
	"noplanner/backend/document/modules/llm"
	"noplanner/backend/document/modules/parser"
	"noplanner/backend/document/modules/rpcerror"
	"noplanner/backend/document/modules/webfetch"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	documentv1 "noplanner/backend/proto/dist/golang/document/v1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
	documentv1.UnimplementedDocumentServiceServer
	store    documentdomain.Store
	blobs    blobstore.Store
	web      webfetch.Fetcher
	analyzer llm.Extractor
	name     string
	version  string
}

func New(store documentdomain.Store, blobs blobstore.Store, web webfetch.Fetcher, analyzer llm.Extractor, name, version string) *Service {
	return &Service{store: store, blobs: blobs, web: web, analyzer: analyzer, name: name, version: version}
}

func (s *Service) GetStatus(context.Context, *commonv1.StatusRequest) (*commonv1.StatusResponse, error) {
	return &commonv1.StatusResponse{Service: s.name, Version: s.version, State: "ready"}, nil
}

func (s *Service) CreateDocument(ctx context.Context, request *documentv1.CreateDocumentRequest) (*documentv1.DocumentVersion, error) {
	return s.create(ctx, request.GetProjectId(), request.GetFileName(), request.GetMediaType(), request.GetOriginal(), "")
}

func (s *Service) CreateDocumentFromUrl(ctx context.Context, request *documentv1.CreateDocumentFromUrlRequest) (*documentv1.DocumentVersion, error) {
	if request.GetProjectId() == "" || request.GetUrl() == "" {
		return nil, mapError(ctx, documentdomain.ErrInvalid)
	}
	page, err := s.web.Fetch(ctx, request.GetUrl())
	if err != nil {
		return nil, mapError(ctx, err)
	}
	return s.create(ctx, request.GetProjectId(), page.FileName, page.MediaType, page.Content, page.URL)
}

func (s *Service) create(ctx context.Context, projectID, fileName, mediaType string, original []byte, sourceURL string) (*documentv1.DocumentVersion, error) {
	if err := validateOriginal(projectID, fileName, mediaType, original); err != nil {
		return nil, mapError(ctx, err)
	}
	hash, key := originalIdentity(original)
	extraction, err := parser.Extract(fileName, mediaType, original)
	if err != nil {
		return nil, mapError(ctx, err)
	}
	if err := s.blobs.Put(ctx, key, mediaType, original); err != nil {
		return nil, mapError(ctx, err)
	}
	input := versionInput(fileName, mediaType, original, hash, key, extraction)
	input.SourceURL = sourceURL
	value, err := s.store.Create(ctx, projectID, input)
	if err != nil {
		return nil, mapError(ctx, err)
	}
	return versionMessage(value, nil), nil
}

func (s *Service) AddDocumentVersion(ctx context.Context, request *documentv1.AddDocumentVersionRequest) (*documentv1.DocumentVersion, error) {
	if err := validateOriginal(request.GetDocumentId(), request.GetFileName(), request.GetMediaType(), request.GetOriginal()); err != nil {
		return nil, mapError(ctx, err)
	}
	hash, key := originalIdentity(request.GetOriginal())
	extraction, err := parser.Extract(request.GetFileName(), request.GetMediaType(), request.GetOriginal())
	if err != nil {
		return nil, mapError(ctx, err)
	}
	if err := s.blobs.Put(ctx, key, request.GetMediaType(), request.GetOriginal()); err != nil {
		return nil, mapError(ctx, err)
	}
	value, err := s.store.AddVersion(ctx, request.GetDocumentId(), versionInput(request.GetFileName(), request.GetMediaType(), request.GetOriginal(), hash, key, extraction))
	if err != nil {
		return nil, mapError(ctx, err)
	}
	return versionMessage(value, nil), nil
}

func (s *Service) GetDocumentVersion(ctx context.Context, request *documentv1.GetDocumentVersionRequest) (*documentv1.DocumentVersion, error) {
	if request.GetDocumentId() == "" || request.GetVersion() == 0 {
		return nil, mapError(ctx, documentdomain.ErrInvalid)
	}
	value, err := s.store.GetVersion(ctx, request.GetDocumentId(), request.GetVersion())
	if err != nil {
		return nil, mapError(ctx, err)
	}
	original, err := s.blobs.Get(ctx, value.ObjectKey)
	if err != nil {
		return nil, mapError(ctx, err)
	}
	hash, _ := originalIdentity(original)
	if hash != value.SHA256 || int64(len(original)) != value.SizeBytes {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "저장된 원문의 무결성을 확인하지 못했습니다.", false)
	}
	return versionMessage(value, original), nil
}

func (s *Service) ListDocumentVersions(ctx context.Context, request *documentv1.ListDocumentVersionsRequest) (*documentv1.ListDocumentVersionsResponse, error) {
	if request.GetDocumentId() == "" {
		return nil, mapError(ctx, documentdomain.ErrInvalid)
	}
	values, err := s.store.ListVersions(ctx, request.GetDocumentId())
	if err != nil {
		return nil, mapError(ctx, err)
	}
	response := &documentv1.ListDocumentVersionsResponse{Versions: make([]*documentv1.DocumentVersion, 0, len(values))}
	for _, value := range values {
		response.Versions = append(response.Versions, versionMessage(value, nil))
	}
	return response, nil
}

func (s *Service) AnalyzeDocumentVersion(ctx context.Context, request *documentv1.AnalyzeDocumentVersionRequest) (*documentv1.AnalyzeDocumentVersionResponse, error) {
	if request.GetDocumentId() == "" || request.GetVersion() == 0 || outputLanguage(request.GetOutputLanguage()) == "" {
		return nil, mapError(ctx, documentdomain.ErrInvalid)
	}
	version, err := s.store.GetVersion(ctx, request.GetDocumentId(), request.GetVersion())
	if err != nil {
		return nil, mapError(ctx, err)
	}
	prompt := analysisPrompt(version, outputLanguage(request.GetOutputLanguage()))
	extracted, err := s.analyzer.Extract(ctx, prompt)
	if err != nil {
		return nil, mapError(ctx, err)
	}
	elements := make([]documentdomain.Element, 0, len(extracted))
	for index, value := range extracted {
		if value.SourceBlockOrdinal == 0 || int(value.SourceBlockOrdinal) > len(version.Extraction.Blocks) || value.Confidence < 0 || value.Confidence > 1 || !validElementType(value.Type) || strings.TrimSpace(value.Content) == "" {
			return nil, mapError(ctx, fmt.Errorf("%w: invalid element at index %d", llm.ErrInvalidResponse, index))
		}
		block := version.Extraction.Blocks[value.SourceBlockOrdinal-1]
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s", version.DocumentID, version.Number, index, value.Content)))
		elements = append(elements, documentdomain.Element{ID: hex.EncodeToString(digest[:8]), Type: value.Type, Content: strings.TrimSpace(value.Content), Confidence: value.Confidence, SourceBlockOrdinal: value.SourceBlockOrdinal, SourceLocation: block.Location})
	}
	if err := s.store.ReplaceElements(ctx, version.DocumentID, version.Number, s.analyzer.Model(), elements); err != nil {
		return nil, mapError(ctx, err)
	}
	response := &documentv1.AnalyzeDocumentVersionResponse{Model: s.analyzer.Model()}
	for _, element := range elements {
		response.Elements = append(response.Elements, elementMessage(element))
	}
	return response, nil
}

func (s *Service) AnalyzeRequirements(ctx context.Context, request *documentv1.AnalyzeDocumentVersionRequest) (*documentv1.AnalyzeRequirementsResponse, error) {
	if request.GetDocumentId() == "" || request.GetVersion() == 0 || outputLanguage(request.GetOutputLanguage()) == "" {
		return nil, mapError(ctx, documentdomain.ErrInvalid)
	}
	version, err := s.store.GetVersion(ctx, request.GetDocumentId(), request.GetVersion())
	if err != nil {
		return nil, mapError(ctx, err)
	}
	extractor, ok := s.analyzer.(llm.RequirementExtractor)
	if !ok {
		return nil, mapError(ctx, llm.ErrConfiguration)
	}
	prompt := analysisPrompt(version, outputLanguage(request.GetOutputLanguage()))
	analysis, err := extractor.ExtractRequirements(ctx, prompt)
	if err != nil {
		return nil, mapError(ctx, err)
	}
	requirements := make([]documentdomain.Requirement, 0, len(analysis.Requirements))
	ids := map[string]string{}
	for index, value := range analysis.Requirements {
		if value.Key == "" || ids[value.Key] != "" || !validRequirementType(value.Type) || !validAnalysisValue(value.Content, value.Confidence, value.SourceBlockOrdinal, len(version.Extraction.Blocks)) {
			return nil, mapError(ctx, fmt.Errorf("%w: invalid requirement at index %d", llm.ErrInvalidResponse, index))
		}
		id := analysisID(version, index, value.Content)
		ids[value.Key] = id
		block := version.Extraction.Blocks[value.SourceBlockOrdinal-1]
		requirements = append(requirements, documentdomain.Requirement{ID: id, Type: value.Type, Content: strings.TrimSpace(value.Content), Confidence: value.Confidence, SourceBlockOrdinal: value.SourceBlockOrdinal, SourceLocation: block.Location})
	}
	dependencies := make([]documentdomain.Dependency, 0, len(analysis.Dependencies))
	for index, value := range analysis.Dependencies {
		if !validDependencyType(value.Type) || !validAnalysisValue(value.Content, value.Confidence, value.SourceBlockOrdinal, len(version.Extraction.Blocks)) {
			return nil, mapError(ctx, fmt.Errorf("%w: invalid dependency at index %d", llm.ErrInvalidResponse, index))
		}
		links := make([]string, 0, len(value.RequirementKeys))
		for _, key := range value.RequirementKeys {
			id := ids[key]
			if id == "" {
				return nil, mapError(ctx, fmt.Errorf("%w: unknown requirement key at dependency index %d", llm.ErrInvalidResponse, index))
			}
			links = append(links, id)
		}
		block := version.Extraction.Blocks[value.SourceBlockOrdinal-1]
		dependencies = append(dependencies, documentdomain.Dependency{ID: analysisID(version, index+len(requirements), value.Content), Type: value.Type, Content: strings.TrimSpace(value.Content), Confidence: value.Confidence, SourceBlockOrdinal: value.SourceBlockOrdinal, SourceLocation: block.Location, RequirementIDs: links})
	}
	if err := s.store.ReplaceRequirements(ctx, version.DocumentID, version.Number, s.analyzer.Model(), requirements, dependencies); err != nil {
		return nil, mapError(ctx, err)
	}
	response := &documentv1.AnalyzeRequirementsResponse{Model: s.analyzer.Model()}
	for _, v := range requirements {
		response.Requirements = append(response.Requirements, requirementMessage(v))
	}
	for _, v := range dependencies {
		response.Dependencies = append(response.Dependencies, dependencyMessage(v))
	}
	return response, nil
}

func outputLanguage(value commonv1.OutputLanguage) string {
	if value == commonv1.OutputLanguage_OUTPUT_LANGUAGE_KOREAN {
		return "Korean"
	}
	if value == commonv1.OutputLanguage_OUTPUT_LANGUAGE_ENGLISH {
		return "English"
	}
	return ""
}
func analysisPrompt(version documentdomain.Version, language string) string {
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Requested output language: %s. Preserve original block citations and do not translate source locations.\nDocument blocks:\n", language)
	for _, block := range version.Extraction.Blocks {
		fmt.Fprintf(&prompt, "[block:%d]\n%s\n", block.Ordinal, block.Text)
	}
	return prompt.String()
}

func validAnalysisValue(content string, confidence float64, ordinal uint32, count int) bool {
	return strings.TrimSpace(content) != "" && confidence >= 0 && confidence <= 1 && ordinal > 0 && int(ordinal) <= count
}
func validRequirementType(v string) bool {
	switch v {
	case "requirement", "policy", "state", "exception":
		return true
	}
	return false
}
func validDependencyType(v string) bool {
	switch v {
	case "data", "api", "technology", "permission", "resource":
		return true
	}
	return false
}
func analysisID(version documentdomain.Version, index int, content string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s", version.DocumentID, version.Number, index, content)))
	return hex.EncodeToString(digest[:8])
}
func sourceLocation(v documentdomain.Location) *documentv1.SourceLocation {
	return &documentv1.SourceLocation{PageNumber: v.PageNumber, SlideNumber: v.SlideNumber, SheetName: v.SheetName, SectionPath: append([]string(nil), v.SectionPath...), ParagraphNumber: v.ParagraphNumber}
}
func requirementMessage(v documentdomain.Requirement) *documentv1.Requirement {
	return &documentv1.Requirement{Id: v.ID, Type: map[string]documentv1.RequirementType{"requirement": documentv1.RequirementType_REQUIREMENT_TYPE_REQUIREMENT, "policy": documentv1.RequirementType_REQUIREMENT_TYPE_POLICY, "state": documentv1.RequirementType_REQUIREMENT_TYPE_STATE, "exception": documentv1.RequirementType_REQUIREMENT_TYPE_EXCEPTION}[v.Type], Content: v.Content, Confidence: v.Confidence, SourceBlockOrdinal: v.SourceBlockOrdinal, SourceLocation: sourceLocation(v.SourceLocation)}
}
func dependencyMessage(v documentdomain.Dependency) *documentv1.Dependency {
	return &documentv1.Dependency{Id: v.ID, Type: map[string]documentv1.DependencyType{"data": documentv1.DependencyType_DEPENDENCY_TYPE_DATA, "api": documentv1.DependencyType_DEPENDENCY_TYPE_API, "technology": documentv1.DependencyType_DEPENDENCY_TYPE_TECHNOLOGY, "permission": documentv1.DependencyType_DEPENDENCY_TYPE_PERMISSION, "resource": documentv1.DependencyType_DEPENDENCY_TYPE_RESOURCE}[v.Type], Content: v.Content, Confidence: v.Confidence, SourceBlockOrdinal: v.SourceBlockOrdinal, SourceLocation: sourceLocation(v.SourceLocation), RequirementIds: append([]string(nil), v.RequirementIDs...)}
}

func validElementType(value string) bool {
	switch value {
	case "problem", "purpose", "goal", "target", "claim", "evidence", "assumption", "solution":
		return true
	}
	return false
}

func elementMessage(value documentdomain.Element) *documentv1.DocumentElement {
	return &documentv1.DocumentElement{Id: value.ID, Type: elementType(value.Type), Content: value.Content, Confidence: value.Confidence, SourceBlockOrdinal: value.SourceBlockOrdinal, SourceLocation: &documentv1.SourceLocation{PageNumber: value.SourceLocation.PageNumber, SlideNumber: value.SourceLocation.SlideNumber, SheetName: value.SourceLocation.SheetName, SectionPath: append([]string(nil), value.SourceLocation.SectionPath...), ParagraphNumber: value.SourceLocation.ParagraphNumber}}
}

func elementType(value string) documentv1.DocumentElementType {
	return map[string]documentv1.DocumentElementType{"problem": documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_PROBLEM, "purpose": documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_PURPOSE, "goal": documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_GOAL, "target": documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_TARGET, "claim": documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_CLAIM, "evidence": documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_EVIDENCE, "assumption": documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_ASSUMPTION, "solution": documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_SOLUTION}[value]
}

func validateOriginal(scope, fileName, mediaType string, original []byte) error {
	if scope == "" || fileName == "" || mediaType == "" || len(original) == 0 {
		return documentdomain.ErrInvalid
	}
	return nil
}

func originalIdentity(original []byte) (string, string) {
	digest := sha256.Sum256(original)
	hash := hex.EncodeToString(digest[:])
	return hash, fmt.Sprintf("originals/%s", hash)
}

func versionMessage(value documentdomain.Version, original []byte) *documentv1.DocumentVersion {
	message := &documentv1.DocumentVersion{
		DocumentId: value.DocumentID, ProjectId: value.ProjectID, Version: value.Number,
		FileName: value.FileName, MediaType: value.MediaType, SizeBytes: value.SizeBytes,
		Sha256: value.SHA256, CreatedAt: timestamppb.New(value.CreatedAt), Original: original, ExtractedText: value.Extraction.Text, SourceUrl: value.SourceURL,
		ParseStatus: documentv1.ParseStatus_PARSE_STATUS_COMPLETE,
	}
	if len(value.Extraction.Issues) > 0 {
		message.ParseStatus = documentv1.ParseStatus_PARSE_STATUS_PARTIAL
	}
	for _, issue := range value.Extraction.Issues {
		message.ParseIssues = append(message.ParseIssues, &documentv1.ParseIssue{Scope: issue.Scope, Message: issue.Message})
	}
	for _, block := range value.Extraction.Blocks {
		message.Blocks = append(message.Blocks, &documentv1.DocumentBlock{
			Type: blockType(block.Type), Text: block.Text, Ordinal: block.Ordinal, HeadingLevel: block.HeadingLevel,
			Location: &documentv1.SourceLocation{
				PageNumber: block.Location.PageNumber, SlideNumber: block.Location.SlideNumber, SheetName: block.Location.SheetName,
				SectionPath: append([]string(nil), block.Location.SectionPath...), ParagraphNumber: block.Location.ParagraphNumber,
			},
		})
	}
	return message
}

func versionInput(fileName, mediaType string, original []byte, hash, key string, extraction parser.Result) documentdomain.VersionInput {
	input := documentdomain.VersionInput{FileName: fileName, MediaType: mediaType, SizeBytes: int64(len(original)), SHA256: hash, ObjectKey: key}
	input.Extraction.Text = extraction.Text
	for _, issue := range extraction.Issues {
		input.Extraction.Issues = append(input.Extraction.Issues, documentdomain.ParseIssue{Scope: issue.Scope, Message: issue.Message})
	}
	for _, block := range extraction.Blocks {
		input.Extraction.Blocks = append(input.Extraction.Blocks, documentdomain.Block{
			Type: block.Type, Text: block.Text, Ordinal: block.Ordinal, HeadingLevel: block.HeadingLevel,
			Location: documentdomain.Location{
				PageNumber: block.Location.PageNumber, SlideNumber: block.Location.SlideNumber, SheetName: block.Location.SheetName,
				SectionPath: append([]string(nil), block.Location.SectionPath...), ParagraphNumber: block.Location.ParagraphNumber,
			},
		})
	}
	return input
}

func blockType(value string) documentv1.DocumentBlockType {
	switch value {
	case "heading":
		return documentv1.DocumentBlockType_DOCUMENT_BLOCK_TYPE_HEADING
	case "paragraph":
		return documentv1.DocumentBlockType_DOCUMENT_BLOCK_TYPE_PARAGRAPH
	case "page":
		return documentv1.DocumentBlockType_DOCUMENT_BLOCK_TYPE_PAGE
	case "slide":
		return documentv1.DocumentBlockType_DOCUMENT_BLOCK_TYPE_SLIDE
	case "sheet":
		return documentv1.DocumentBlockType_DOCUMENT_BLOCK_TYPE_SHEET
	case "image_ocr":
		return documentv1.DocumentBlockType_DOCUMENT_BLOCK_TYPE_IMAGE_OCR
	case "web_section":
		return documentv1.DocumentBlockType_DOCUMENT_BLOCK_TYPE_WEB_SECTION
	default:
		return documentv1.DocumentBlockType_DOCUMENT_BLOCK_TYPE_UNSPECIFIED
	}
}

func mapError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, documentdomain.ErrInvalid):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "문서 입력값을 확인해 주세요.", false)
	case errors.Is(err, documentdomain.ErrNotFound):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_NOT_FOUND, "문서 버전을 찾을 수 없습니다.", false)
	case errors.Is(err, parser.ErrUnsupported):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "지원하지 않는 문서 형식입니다.", false)
	case errors.Is(err, parser.ErrInvalid):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "문서 내용을 추출하지 못했습니다.", false)
	case errors.Is(err, parser.ErrOCRUnavailable):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_CONFIGURATION_REQUIRED, "로컬 OCR 실행 환경이 필요합니다.", false)
	case errors.Is(err, webfetch.ErrUnsafeURL):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "내부망 또는 허용되지 않은 URL에는 접근할 수 없습니다.", false)
	case errors.Is(err, webfetch.ErrInvalidPage):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "웹페이지 내용을 가져오지 못했습니다.", false)
	case errors.Is(err, llm.ErrConfiguration):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_CONFIGURATION_REQUIRED, "문서 분석 LLM 설정이 필요합니다.", false)
	case errors.Is(err, llm.ErrInvalidResponse):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_DEPENDENCY_UNAVAILABLE, "문서 분석 결과를 검증하지 못했습니다: "+err.Error(), true)
	case errors.Is(err, llm.ErrUnavailable):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_DEPENDENCY_UNAVAILABLE, "문서 분석 LLM을 일시적으로 사용할 수 없습니다.", true)
	default:
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_DEPENDENCY_UNAVAILABLE, "문서 저장소를 사용할 수 없습니다.", true)
	}
}
