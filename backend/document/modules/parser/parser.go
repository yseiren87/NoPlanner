package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"
	"golang.org/x/net/html"
)

var (
	ErrUnsupported    = errors.New("unsupported document format")
	ErrInvalid        = errors.New("invalid document content")
	ErrOCRUnavailable = errors.New("ocr engine is unavailable")
)

type Block struct {
	Type         string
	Text         string
	Ordinal      uint32
	HeadingLevel uint32
	Location     Location
}

type Location struct {
	PageNumber      uint32
	SlideNumber     uint32
	SheetName       string
	SectionPath     []string
	ParagraphNumber uint32
}

type Result struct {
	Text   string
	Blocks []Block
	Issues []Issue
}

type Issue struct {
	Scope   string
	Message string
}

const (
	maxArchiveFiles            = 1000
	maxArchiveUncompressedSize = 64 << 20
)

func Extract(fileName, mediaType string, content []byte) (Result, error) {
	format := detectFormat(fileName, mediaType)
	switch format {
	case "text":
		return extractText(content)
	case "markdown":
		return extractMarkdown(content)
	case "docx":
		return extractDOCX(content)
	case "pdf":
		return extractPDF(content)
	case "pptx":
		return extractPPTX(content)
	case "xlsx":
		return extractXLSX(content)
	case "image":
		return extractImage(content)
	case "html":
		return extractHTML(content)
	default:
		return Result{}, ErrUnsupported
	}
}

func detectFormat(fileName, mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "text/plain":
		return "text"
	case "text/markdown", "text/x-markdown":
		return "markdown"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"
	case "application/pdf":
		return "pdf"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return "pptx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "xlsx"
	case "image/png", "image/jpeg", "image/gif":
		return "image"
	case "text/html", "application/xhtml+xml":
		return "html"
	}
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".txt":
		return "text"
	case ".md", ".markdown":
		return "markdown"
	case ".docx":
		return "docx"
	case ".pdf":
		return "pdf"
	case ".pptx":
		return "pptx"
	case ".xlsx":
		return "xlsx"
	case ".png", ".jpg", ".jpeg", ".gif":
		return "image"
	case ".html", ".htm":
		return "html"
	default:
		return ""
	}
}

func extractPPTX(content []byte) (Result, error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil || !safeArchive(archive) {
		return Result{}, ErrInvalid
	}
	slides := make([]*zip.File, 0)
	for _, file := range archive.File {
		if strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml") {
			slides = append(slides, file)
		}
	}
	sort.Slice(slides, func(i, j int) bool { return slideNumber(slides[i].Name) < slideNumber(slides[j].Name) })
	blocks := make([]Block, 0, len(slides))
	issues := make([]Issue, 0)
	for _, slide := range slides {
		number := slideNumber(slide.Name)
		reader, err := slide.Open()
		if err != nil {
			issues = append(issues, Issue{Scope: fmt.Sprintf("slide:%d", number), Message: "슬라이드 파일을 열 수 없습니다."})
			continue
		}
		text, err := xmlText(reader, "t")
		reader.Close()
		if err != nil {
			issues = append(issues, Issue{Scope: fmt.Sprintf("slide:%d", number), Message: "슬라이드 XML을 해석할 수 없습니다."})
			continue
		}
		if text != "" {
			blocks = append(blocks, Block{Type: "slide", Text: text, Location: Location{SlideNumber: uint32(number)}})
		} else {
			issues = append(issues, Issue{Scope: fmt.Sprintf("slide:%d", number), Message: "추출할 텍스트가 없습니다."})
		}
	}
	return finishWithIssues(blocks, issues)
}

func safeArchive(archive *zip.Reader) bool {
	if len(archive.File) > maxArchiveFiles {
		return false
	}
	var total uint64
	for _, file := range archive.File {
		total += file.UncompressedSize64
		if total > maxArchiveUncompressedSize {
			return false
		}
	}
	return true
}

func slideNumber(name string) int {
	base := strings.TrimSuffix(filepath.Base(name), ".xml")
	value, _ := strconv.Atoi(strings.TrimPrefix(base, "slide"))
	return value
}

func xmlText(reader io.Reader, element string) (string, error) {
	decoder := xml.NewDecoder(reader)
	values := make([]string, 0)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == element {
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return "", err
			}
			if value = strings.TrimSpace(value); value != "" {
				values = append(values, value)
			}
		}
	}
	return strings.Join(values, "\n"), nil
}

func extractXLSX(content []byte) (Result, error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil || !safeArchive(archive) {
		return Result{}, ErrInvalid
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return Result{}, ErrInvalid
	}
	defer workbook.Close()
	blocks := make([]Block, 0)
	issues := make([]Issue, 0)
	for _, sheet := range workbook.GetSheetList() {
		rows, err := workbook.GetRows(sheet)
		if err != nil {
			issues = append(issues, Issue{Scope: "sheet:" + sheet, Message: "시트 내용을 해석할 수 없습니다."})
			continue
		}
		lines := make([]string, 0, len(rows)+1)
		lines = append(lines, sheet)
		for _, row := range rows {
			if value := strings.TrimSpace(strings.Join(row, "\t")); value != "" {
				lines = append(lines, value)
			}
		}
		if len(lines) > 1 {
			blocks = append(blocks, Block{Type: "sheet", Text: strings.Join(lines, "\n"), Location: Location{SheetName: sheet}})
		}
	}
	return finishWithIssues(blocks, issues)
}

var runOCR = commandOCR

func extractImage(content []byte) (Result, error) {
	if _, _, err := image.DecodeConfig(bytes.NewReader(content)); err != nil {
		return Result{}, ErrInvalid
	}
	text, err := runOCR(content)
	if err != nil {
		return Result{}, err
	}
	return finish([]Block{{Type: "image_ocr", Text: strings.TrimSpace(text), Location: Location{ParagraphNumber: 1}}})
}

func commandOCR(content []byte) (string, error) {
	path, err := exec.LookPath("tesseract")
	if err != nil {
		return "", ErrOCRUnavailable
	}
	file, err := os.CreateTemp("", "noplanner-ocr-*.png")
	if err != nil {
		return "", err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(content); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	output, err := exec.Command(path, name, "stdout", "-l", "kor+eng").Output()
	if err != nil {
		return "", ErrInvalid
	}
	return string(output), nil
}

func extractHTML(content []byte) (Result, error) {
	root, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		return Result{}, ErrInvalid
	}
	blocks := make([]Block, 0)
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			name := strings.ToLower(node.Data)
			kind, level := "", uint32(0)
			switch name {
			case "h1", "h2", "h3", "h4", "h5", "h6":
				kind = "heading"
				parsed, _ := strconv.Atoi(strings.TrimPrefix(name, "h"))
				level = uint32(parsed)
			case "p", "li", "td", "th":
				kind = "web_section"
			}
			if kind != "" {
				if text := strings.TrimSpace(nodeText(node)); text != "" {
					blocks = append(blocks, Block{Type: kind, Text: text, HeadingLevel: level})
					return
				}
			}
			if name == "script" || name == "style" || name == "noscript" {
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return finish(blocks)
}

func nodeText(node *html.Node) string {
	values := make([]string, 0)
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			if value := strings.TrimSpace(current.Data); value != "" {
				values = append(values, value)
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(values, " ")
}

func extractText(content []byte) (Result, error) {
	if !utf8.Valid(content) {
		return Result{}, ErrInvalid
	}
	text := strings.TrimSpace(strings.TrimPrefix(string(content), "\ufeff"))
	return paragraphs(text)
}

func extractMarkdown(content []byte) (Result, error) {
	if !utf8.Valid(content) {
		return Result{}, ErrInvalid
	}
	lines := strings.Split(strings.TrimSpace(strings.TrimPrefix(string(content), "\ufeff")), "\n")
	blocks := make([]Block, 0)
	paragraph := make([]string, 0)
	flush := func() {
		text := strings.TrimSpace(strings.Join(paragraph, " "))
		if text != "" {
			blocks = append(blocks, Block{Type: "paragraph", Text: text})
		}
		paragraph = paragraph[:0]
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		level := markdownHeadingLevel(trimmed)
		if level > 0 {
			flush()
			blocks = append(blocks, Block{Type: "heading", Text: strings.TrimSpace(trimmed[level:]), HeadingLevel: uint32(level)})
			continue
		}
		paragraph = append(paragraph, trimmed)
	}
	flush()
	return finish(blocks)
}

func markdownHeadingLevel(line string) int {
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || line[level] != ' ' {
		return 0
	}
	return level
}

func paragraphs(text string) (Result, error) {
	parts := strings.FieldsFunc(strings.ReplaceAll(text, "\r\n", "\n"), func(r rune) bool { return r == '\n' })
	blocks := make([]Block, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			blocks = append(blocks, Block{Type: "paragraph", Text: value})
		}
	}
	return finish(blocks)
}

func extractDOCX(content []byte) (Result, error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil || !safeArchive(archive) {
		return Result{}, ErrInvalid
	}
	var source io.ReadCloser
	for _, file := range archive.File {
		if file.Name == "word/document.xml" {
			source, err = file.Open()
			break
		}
	}
	if err != nil || source == nil {
		return Result{}, ErrInvalid
	}
	defer source.Close()
	decoder := xml.NewDecoder(source)
	blocks := make([]Block, 0)
	var text strings.Builder
	var style string
	inParagraph := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return finishWithIssues(blocks, []Issue{{Scope: "document.xml", Message: "문서 XML의 일부를 해석할 수 없습니다."}})
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "p":
				inParagraph, style = true, ""
				text.Reset()
			case "pStyle":
				for _, attribute := range value.Attr {
					if attribute.Name.Local == "val" {
						style = attribute.Value
					}
				}
			case "t":
				if inParagraph {
					var fragment string
					if err := decoder.DecodeElement(&fragment, &value); err != nil {
						return finishWithIssues(blocks, []Issue{{Scope: "document.xml", Message: "문서 XML의 일부를 해석할 수 없습니다."}})
					}
					text.WriteString(fragment)
				}
			case "tab":
				text.WriteByte('\t')
			case "br":
				text.WriteByte('\n')
			}
		case xml.EndElement:
			if value.Name.Local == "p" && inParagraph {
				paragraph := strings.TrimSpace(text.String())
				if paragraph != "" {
					level := docxHeadingLevel(style)
					kind := "paragraph"
					if level > 0 {
						kind = "heading"
					}
					blocks = append(blocks, Block{Type: kind, Text: paragraph, HeadingLevel: level})
				}
				inParagraph = false
			}
		}
	}
	return finish(blocks)
}

func docxHeadingLevel(style string) uint32 {
	lower := strings.ToLower(style)
	for _, prefix := range []string{"heading", "제목"} {
		if strings.HasPrefix(lower, prefix) {
			level, _ := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(lower, prefix)), 10, 32)
			if level >= 1 && level <= 9 {
				return uint32(level)
			}
		}
	}
	return 0
}

func extractPDF(content []byte) (Result, error) {
	file, err := os.CreateTemp("", "noplanner-*.pdf")
	if err != nil {
		return Result{}, err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.Write(content); err != nil {
		file.Close()
		return Result{}, err
	}
	if err := file.Close(); err != nil {
		return Result{}, err
	}
	handle, reader, err := pdf.Open(path)
	if err != nil {
		return Result{}, ErrInvalid
	}
	defer handle.Close()
	blocks := make([]Block, 0, reader.NumPage())
	issues := make([]Issue, 0)
	for pageNumber := 1; pageNumber <= reader.NumPage(); pageNumber++ {
		page := reader.Page(pageNumber)
		rows, err := page.GetTextByRow()
		if err != nil {
			issues = append(issues, Issue{Scope: fmt.Sprintf("page:%d", pageNumber), Message: "페이지 텍스트를 해석할 수 없습니다."})
			continue
		}
		lines := make([]string, 0, len(rows))
		for _, row := range rows {
			words := make([]string, 0, len(row.Content))
			for _, word := range row.Content {
				words = append(words, word.S)
			}
			if line := strings.TrimSpace(strings.Join(words, " ")); line != "" {
				lines = append(lines, line)
			}
		}
		if text := strings.TrimSpace(strings.Join(lines, "\n")); text != "" {
			blocks = append(blocks, Block{Type: "page", Text: text, Location: Location{PageNumber: uint32(pageNumber)}})
		} else {
			issues = append(issues, Issue{Scope: fmt.Sprintf("page:%d", pageNumber), Message: "텍스트를 찾지 못했습니다. 스캔 페이지라면 OCR이 필요합니다."})
		}
	}
	return finishWithIssues(blocks, issues)
}

func finish(blocks []Block) (Result, error) {
	return finishWithIssues(blocks, nil)
}

func finishWithIssues(blocks []Block, issues []Issue) (Result, error) {
	if len(blocks) == 0 && len(issues) == 0 {
		return Result{}, ErrInvalid
	}
	texts := make([]string, 0, len(blocks))
	sections := make([]string, 0, 6)
	var paragraphNumber uint32
	for index := range blocks {
		blocks[index].Ordinal = uint32(index + 1)
		if blocks[index].Type == "heading" && blocks[index].HeadingLevel > 0 {
			level := int(blocks[index].HeadingLevel)
			if level <= len(sections) {
				sections = sections[:level-1]
			}
			for len(sections) < level-1 {
				sections = append(sections, "")
			}
			sections = append(sections, blocks[index].Text)
			blocks[index].Location.SectionPath = compactPath(sections)
		} else if blocks[index].Location.PageNumber == 0 && blocks[index].Location.SlideNumber == 0 && blocks[index].Location.SheetName == "" {
			paragraphNumber++
			if blocks[index].Location.ParagraphNumber == 0 {
				blocks[index].Location.ParagraphNumber = paragraphNumber
			}
			blocks[index].Location.SectionPath = compactPath(sections)
		}
		texts = append(texts, blocks[index].Text)
	}
	return Result{Text: strings.Join(texts, "\n\n"), Blocks: blocks, Issues: issues}, nil
}

func compactPath(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
