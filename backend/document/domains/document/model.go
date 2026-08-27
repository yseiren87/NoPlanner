package document

import "time"

type Version struct {
	DocumentID string
	ProjectID  string
	Number     uint32
	FileName   string
	MediaType  string
	SizeBytes  int64
	SHA256     string
	ObjectKey  string
	CreatedAt  time.Time
	Extraction Extraction
	SourceURL  string
}

type Element struct {
	ID                 string
	Type               string
	Content            string
	Confidence         float64
	SourceBlockOrdinal uint32
	SourceLocation     Location
}

type Requirement struct {
	ID, Type, Content  string
	Confidence         float64
	SourceBlockOrdinal uint32
	SourceLocation     Location
}
type Dependency struct {
	ID, Type, Content  string
	Confidence         float64
	SourceBlockOrdinal uint32
	SourceLocation     Location
	RequirementIDs     []string
}

type Block struct {
	Type         string   `json:"type"`
	Text         string   `json:"text"`
	Ordinal      uint32   `json:"ordinal"`
	HeadingLevel uint32   `json:"heading_level,omitempty"`
	Location     Location `json:"location"`
}

type Location struct {
	PageNumber      uint32   `json:"page_number,omitempty"`
	SlideNumber     uint32   `json:"slide_number,omitempty"`
	SheetName       string   `json:"sheet_name,omitempty"`
	SectionPath     []string `json:"section_path,omitempty"`
	ParagraphNumber uint32   `json:"paragraph_number,omitempty"`
}

type Extraction struct {
	Text   string       `json:"text"`
	Blocks []Block      `json:"blocks"`
	Issues []ParseIssue `json:"issues"`
}

type ParseIssue struct {
	Scope   string `json:"scope"`
	Message string `json:"message"`
}

type VersionInput struct {
	FileName   string
	MediaType  string
	SizeBytes  int64
	SHA256     string
	ObjectKey  string
	Extraction Extraction
	SourceURL  string
}
