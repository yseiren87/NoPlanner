package parser

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"
)

func TestExtractTextAndMarkdown(t *testing.T) {
	plain, err := Extract("plan.txt", "text/plain", []byte("Purpose\n\nRequirement"))
	if err != nil || plain.Text != "Purpose\n\nRequirement" || len(plain.Blocks) != 2 {
		t.Fatalf("plain extraction = %#v, %v", plain, err)
	}
	if plain.Blocks[0].Location.ParagraphNumber != 1 || plain.Blocks[1].Location.ParagraphNumber != 2 {
		t.Fatalf("plain locations = %#v", plain.Blocks)
	}
	markdown, err := Extract("plan.md", "text/markdown", []byte("# Purpose\n\nBuild a verifier.\n\n## Scope\n\nDocuments"))
	if err != nil || len(markdown.Blocks) != 4 {
		t.Fatalf("markdown extraction = %#v, %v", markdown, err)
	}
	if markdown.Blocks[0].Type != "heading" || markdown.Blocks[0].HeadingLevel != 1 || markdown.Blocks[2].HeadingLevel != 2 {
		t.Fatalf("markdown headings = %#v", markdown.Blocks)
	}
	if strings.Join(markdown.Blocks[3].Location.SectionPath, "/") != "Purpose/Scope" || markdown.Blocks[3].Location.ParagraphNumber != 2 {
		t.Fatalf("markdown location = %#v", markdown.Blocks[3].Location)
	}
}

func TestExtractDOCXParagraphsAndHeadings(t *testing.T) {
	var content bytes.Buffer
	archive := zip.NewWriter(&content)
	document, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = document.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Purpose</w:t></w:r></w:p>
<w:p><w:r><w:t>Verify the plan.</w:t></w:r></w:p>
</w:body></w:document>`))
	if err != nil || archive.Close() != nil {
		t.Fatal(err)
	}
	result, err := Extract("plan.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", content.Bytes())
	if err != nil || len(result.Blocks) != 2 || result.Blocks[0].Type != "heading" || result.Blocks[0].HeadingLevel != 1 {
		t.Fatalf("docx extraction = %#v, %v", result, err)
	}
	if strings.Join(result.Blocks[1].Location.SectionPath, "/") != "Purpose" || result.Blocks[1].Location.ParagraphNumber != 1 {
		t.Fatalf("docx location = %#v", result.Blocks[1].Location)
	}
}

func TestExtractPDFByPage(t *testing.T) {
	document := fpdf.New("P", "mm", "A4", "")
	document.AddPage()
	document.SetFont("Arial", "", 12)
	document.Cell(40, 10, "Executable plan")
	var content bytes.Buffer
	if err := document.Output(&content); err != nil {
		t.Fatal(err)
	}
	result, err := Extract("plan.pdf", "application/pdf", content.Bytes())
	if err != nil || len(result.Blocks) != 1 || result.Blocks[0].Type != "page" || !strings.Contains(result.Text, "Executable plan") {
		t.Fatalf("pdf extraction = %#v, %v", result, err)
	}
	if result.Blocks[0].Location.PageNumber != 1 {
		t.Fatalf("pdf location = %#v", result.Blocks[0].Location)
	}
}

func TestExtractScannedOrBlankPDFReportsPageScope(t *testing.T) {
	document := fpdf.New("P", "mm", "A4", "")
	document.AddPage()
	var content bytes.Buffer
	if err := document.Output(&content); err != nil {
		t.Fatal(err)
	}
	result, err := Extract("scan.pdf", "application/pdf", content.Bytes())
	if err != nil || len(result.Blocks) != 0 || len(result.Issues) != 1 {
		t.Fatalf("blank pdf extraction = %#v, %v", result, err)
	}
	if result.Issues[0].Scope != "page:1" || !strings.Contains(result.Issues[0].Message, "OCR") {
		t.Fatalf("blank pdf issue = %#v", result.Issues[0])
	}
}

func TestExtractRejectsUnsupportedAndMalformedFiles(t *testing.T) {
	if _, err := Extract("plan.bin", "application/octet-stream", []byte("value")); err != ErrUnsupported {
		t.Fatalf("unsupported error = %v", err)
	}
	if _, err := Extract("plan.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", []byte("not a zip")); err != ErrInvalid {
		t.Fatalf("invalid docx error = %v", err)
	}
}

func TestExtractPPTXSlides(t *testing.T) {
	var content bytes.Buffer
	archive := zip.NewWriter(&content)
	for name, value := range map[string]string{
		"ppt/slides/slide2.xml": `<p:sld xmlns:p="p" xmlns:a="a"><a:t>Second slide</a:t></p:sld>`,
		"ppt/slides/slide1.xml": `<p:sld xmlns:p="p" xmlns:a="a"><a:t>First slide</a:t></p:sld>`,
	} {
		file, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := Extract("plan.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", content.Bytes())
	if err != nil || len(result.Blocks) != 2 || result.Blocks[0].Text != "First slide" || result.Blocks[1].Type != "slide" {
		t.Fatalf("pptx extraction = %#v, %v", result, err)
	}
	if result.Blocks[0].Location.SlideNumber != 1 || result.Blocks[1].Location.SlideNumber != 2 {
		t.Fatalf("pptx locations = %#v", result.Blocks)
	}
}

func TestExtractPPTXPreservesValidSlidesWhenOneIsMalformed(t *testing.T) {
	var content bytes.Buffer
	archive := zip.NewWriter(&content)
	for name, value := range map[string]string{
		"ppt/slides/slide1.xml": `<p:sld xmlns:p="p" xmlns:a="a"><a:t>Valid slide</a:t></p:sld>`,
		"ppt/slides/slide2.xml": `<p:sld xmlns:p="p" xmlns:a="a"><a:t>broken`,
	} {
		file, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := Extract("partial.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", content.Bytes())
	if err != nil || len(result.Blocks) != 1 || result.Blocks[0].Text != "Valid slide" || len(result.Issues) != 1 || result.Issues[0].Scope != "slide:2" {
		t.Fatalf("partial pptx extraction = %#v, %v", result, err)
	}
}

func TestExtractDOCXPreservesCompletedParagraphsWhenXMLIsMalformed(t *testing.T) {
	var content bytes.Buffer
	archive := zip.NewWriter(&content)
	document, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = document.Write([]byte(`<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>Preserved</w:t></w:r></w:p><w:p><w:r>`))
	if err != nil || archive.Close() != nil {
		t.Fatal(err)
	}
	result, err := Extract("partial.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", content.Bytes())
	if err != nil || len(result.Blocks) != 1 || result.Blocks[0].Text != "Preserved" || len(result.Issues) != 1 || result.Issues[0].Scope != "document.xml" {
		t.Fatalf("partial docx extraction = %#v, %v", result, err)
	}
}

func TestExtractRejectsArchiveWithExcessiveFileCount(t *testing.T) {
	var content bytes.Buffer
	archive := zip.NewWriter(&content)
	for index := 0; index <= maxArchiveFiles; index++ {
		if _, err := archive.Create(fmt.Sprintf("unused/%d", index)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract("oversized.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", content.Bytes()); err != ErrInvalid {
		t.Fatalf("archive limit error = %v", err)
	}
}

func TestExtractXLSXSheets(t *testing.T) {
	workbook := excelize.NewFile()
	if err := workbook.SetCellValue("Sheet1", "A1", "Metric"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCellValue("Sheet1", "B1", 42); err != nil {
		t.Fatal(err)
	}
	var content bytes.Buffer
	if err := workbook.Write(&content); err != nil {
		t.Fatal(err)
	}
	result, err := Extract("data.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content.Bytes())
	if err != nil || len(result.Blocks) != 1 || result.Blocks[0].Type != "sheet" || !strings.Contains(result.Text, "Metric\t42") {
		t.Fatalf("xlsx extraction = %#v, %v", result, err)
	}
	if result.Blocks[0].Location.SheetName != "Sheet1" {
		t.Fatalf("xlsx location = %#v", result.Blocks[0].Location)
	}
}

func TestExtractImageWithOCR(t *testing.T) {
	previous := runOCR
	runOCR = func([]byte) (string, error) { return "recognized plan", nil }
	t.Cleanup(func() { runOCR = previous })
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 2))
	canvas.Set(0, 0, color.Black)
	var content bytes.Buffer
	if err := png.Encode(&content, canvas); err != nil {
		t.Fatal(err)
	}
	result, err := Extract("scan.png", "image/png", content.Bytes())
	if err != nil || result.Text != "recognized plan" || result.Blocks[0].Type != "image_ocr" {
		t.Fatalf("image extraction = %#v, %v", result, err)
	}
	if result.Blocks[0].Location.ParagraphNumber != 1 {
		t.Fatalf("image location = %#v", result.Blocks[0].Location)
	}
}

func TestExtractHTMLStructure(t *testing.T) {
	content := []byte(`<html><head><script>ignore()</script></head><body><h1>Purpose</h1><p>Verify <strong>facts</strong>.</p><ul><li>Evidence</li></ul><table><tr><th>Metric</th><td>42</td></tr></table></body></html>`)
	result, err := Extract("page.html", "text/html", content)
	if err != nil || len(result.Blocks) != 5 || result.Blocks[0].Type != "heading" || strings.Contains(result.Text, "ignore") || !strings.Contains(result.Text, "Metric") || !strings.Contains(result.Text, "42") {
		t.Fatalf("html extraction = %#v, %v", result, err)
	}
	if strings.Join(result.Blocks[1].Location.SectionPath, "/") != "Purpose" || result.Blocks[1].Location.ParagraphNumber != 1 {
		t.Fatalf("html location = %#v", result.Blocks[1].Location)
	}
}
