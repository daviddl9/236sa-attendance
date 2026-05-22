package agent

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// docxToText extracts visible paragraph text from a .docx file by
// reading word/document.xml inside the zip and collecting <w:t> text
// nodes. Paragraphs (<w:p>) are separated by newlines and runs (<w:r>)
// inside a paragraph are concatenated. No external library required.
func docxToText(contents []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		return "", fmt.Errorf("agent: open docx: %w", err)
	}

	var docXML *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docXML = f
			break
		}
	}
	if docXML == nil {
		return "", fmt.Errorf("agent: docx missing word/document.xml")
	}

	rc, err := docXML.Open()
	if err != nil {
		return "", fmt.Errorf("agent: read word/document.xml: %w", err)
	}
	defer rc.Close()

	dec := xml.NewDecoder(rc)
	var b strings.Builder
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("agent: parse docx xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				inText = true
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				b.WriteByte('\n')
			case "tab":
				b.WriteByte('\t')
			}
		case xml.CharData:
			if inText {
				b.Write(t)
			}
		}
	}

	text := strings.TrimSpace(b.String())
	if text == "" {
		return "", ErrNoRowsDetected
	}
	return text, nil
}
