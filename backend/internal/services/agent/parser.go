package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
)

const (
	mediaTypeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	mediaTypeXLSM = "application/vnd.ms-excel.sheet.macroEnabled.12"
	mediaTypeXLS  = "application/vnd.ms-excel"
	mediaTypeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	mediaTypeDOC  = "application/msword"
	mediaTypePDF  = "application/pdf"
)

// DefaultMaxDocumentBytes caps every uploaded document at 10 MB. The
// agent call is bounded but Anthropic itself rejects very large
// requests, and a tight server-side cap makes the rejection path
// uniform.
const DefaultMaxDocumentBytes = 10 * 1024 * 1024

// DefaultParser is the production Parser. It dispatches on media type:
// XLSX/XLSM → text table → text content block; DOCX → text → text
// content block; PDF → base64 → document content block.
type DefaultParser struct {
	client       Client
	maxBytes     int
	model        string
	systemPrompt string
}

// NewParser builds a DefaultParser around the given Client. The model
// is left at "" so the client's default applies.
func NewParser(client Client) *DefaultParser {
	return &DefaultParser{
		client:       client,
		maxBytes:     DefaultMaxDocumentBytes,
		model:        "",
		systemPrompt: defaultSystemPrompt,
	}
}

const defaultSystemPrompt = `You extract personnel rows from a Singapore Armed Forces (SAF) callup or roster document.

For each personnel listed in the document, return a row with:
- full_name: the person's full name exactly as written in NRIC, uppercase, no titles or ranks prepended
- rank: military rank abbreviation (REC, PTE, LCP, CPL, CFC, 3SG, 2SG, 1SG, SSG, MSG, 3WO, 2WO, 1WO, MWO, SWO, 2LT, LTA, CPT, MAJ, LTC) when present; empty string otherwise
- battery: one of HQ, Alpha, Bravo when present in the document; empty string otherwise. Treat "A Bty", "Alpha Bty", "BTY A" as Alpha; "B Bty", "Bravo Bty", "BTY B" as Bravo; "HQ Bty" as HQ
- nric_last5: the last 5 characters of the NRIC (4 digits followed by a single letter, e.g. "1234A"). Uppercase. Empty string if not present
- extras: keep this EMPTY ({}) unless there is a highly distinctive field not captured above (e.g. a callup serial, contact number, or unique sub-unit code). Do NOT copy redundant columns like position, vocation, dates, or attendance status — those bloat the response
- source_snippet: a short verbatim quote (≤ 60 chars) from the document for the row, so admins can verify
- confidence: a number in [0.0, 1.0] where 1.0 means certain of every field, 0.5 means at least one field was inferred or unclear

Rules:
- Skip header rows, totals, and any non-personnel rows.
- Do not invent values. If a field is missing, return an empty string.
- Never invent an NRIC. If NRIC last 5 is not in the document, return an empty string and mark confidence ≤ 0.5.
- Return ALL personnel rows in a single call. Be concise — short snippets, minimal extras.

Call the extract_personnel_rows tool with your output.`

// ParseDocument implements the Parser interface.
func (p *DefaultParser) ParseDocument(ctx context.Context, filename string, contents []byte, mediaType string) ([]ParsedPersonnelRow, error) {
	if len(contents) == 0 {
		return nil, ErrNoRowsDetected
	}
	if p.maxBytes > 0 && len(contents) > p.maxBytes {
		return nil, ErrDocumentTooLarge
	}

	mediaType = normalizeMediaType(mediaType, filename)

	var rows []ParsedPersonnelRow

	switch mediaType {
	case mediaTypeXLSX, mediaTypeXLSM, mediaTypeXLS:
		batches, err := excelToBatches(contents)
		if err != nil {
			return nil, err
		}
		n := len(batches)
		// Cap concurrency at 3 to stay within Anthropic's rate limits.
		const maxConcurrent = 3
		log.Printf("[agent] excel: %d batch(es) of up to %d rows each (concurrency=%d)", n, excelBatchSize, maxConcurrent)

		type batchResult struct {
			rows []ParsedPersonnelRow
			err  error
		}
		results := make([]batchResult, n)
		sem := make(chan struct{}, maxConcurrent)
		var wg sync.WaitGroup
		for i, batchMD := range batches {
			wg.Add(1)
			go func(idx int, md string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				log.Printf("[agent] excel: batch %d/%d starting", idx+1, n)
				batchRows, err := p.callClaude(ctx, []ContentBlock{
					{Type: "text", Text: fmt.Sprintf("Extract personnel rows from this Excel spreadsheet (batch %d/%d, rendered as a markdown table):\n\n%s", idx+1, n, md)},
				})
				if err != nil {
					log.Printf("[agent] excel: batch %d/%d error: %v", idx+1, n, err)
				} else {
					log.Printf("[agent] excel: batch %d/%d done — %d rows", idx+1, n, len(batchRows))
				}
				results[idx] = batchResult{rows: batchRows, err: err}
			}(i, batchMD)
		}
		wg.Wait()

		for _, r := range results {
			if r.err != nil {
				return nil, r.err
			}
			rows = append(rows, r.rows...)
		}

	case mediaTypeDOCX, mediaTypeDOC:
		text, err := docxToText(contents)
		if err != nil {
			return nil, err
		}
		rows, err = p.callClaude(ctx, []ContentBlock{
			{Type: "text", Text: "Extract personnel rows from this Word document text:\n\n" + text},
		})
		if err != nil {
			return nil, err
		}

	case mediaTypePDF:
		var err error
		rows, err = p.callClaude(ctx, []ContentBlock{
			{
				Type: "document",
				Source: &DocumentSource{
					Type:      "base64",
					MediaType: mediaTypePDF,
					Data:      base64.StdEncoding.EncodeToString(contents),
				},
				Title: filename,
			},
			{Type: "text", Text: "Extract personnel rows from the attached PDF document."},
		})
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("agent: unsupported media type %q", mediaType)
	}

	if len(rows) == 0 {
		return nil, ErrNoRowsDetected
	}

	return rows, nil
}

// callClaude sends a single message to Claude and returns the extracted rows.
// An empty rows result is returned as nil (not ErrNoRowsDetected) so batched
// callers can distinguish "no rows in this batch" from a hard error.
func (p *DefaultParser) callClaude(ctx context.Context, userBlocks []ContentBlock) ([]ParsedPersonnelRow, error) {
	req := MessageRequest{
		Model:      p.model,
		System:     p.systemPrompt,
		MaxTokens:  defaultMaxTokens,
		Messages:   []Message{{Role: "user", Content: userBlocks}},
		Tools:      []Tool{personnelTool()},
		ToolChoice: &ToolChoice{Type: "tool", Name: personnelToolName},
	}

	resp, err := p.client.CreateMessage(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.StopReason == "max_tokens" {
		log.Printf("[agent] WARNING: stop_reason=max_tokens — increase defaultMaxTokens or reduce excelBatchSize")
	}

	rows, err := extractRows(resp)
	if err != nil {
		return nil, err
	}
	normalizeRows(rows)
	return rows, nil
}

const personnelToolName = "extract_personnel_rows"

func personnelTool() Tool {
	return Tool{
		Name:        personnelToolName,
		Description: "Return the structured personnel rows extracted from the document.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"rows": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"full_name":      map[string]any{"type": "string"},
							"rank":           map[string]any{"type": "string"},
							"battery":        map[string]any{"type": "string"},
							"nric_last5":     map[string]any{"type": "string"},
							"extras":         map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
							"source_snippet": map[string]any{"type": "string"},
							"confidence":     map[string]any{"type": "number"},
						},
						"required": []string{"full_name", "nric_last5", "confidence"},
					},
				},
			},
			"required": []string{"rows"},
		},
	}
}

// extractRows pulls the tool_use block out of the response and decodes
// its input into our ParsedPersonnelRow slice.
func extractRows(resp *MessageResponse) ([]ParsedPersonnelRow, error) {
	for _, block := range resp.Content {
		if block.Type != "tool_use" || block.Name != personnelToolName {
			continue
		}
		var payload struct {
			Rows []struct {
				FullName      string            `json:"full_name"`
				Rank          string            `json:"rank"`
				Battery       string            `json:"battery"`
				NRICLast5     string            `json:"nric_last5"`
				Extras        map[string]string `json:"extras"`
				SourceSnippet string            `json:"source_snippet"`
				Confidence    float64           `json:"confidence"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(block.Input, &payload); err != nil {
			return nil, fmt.Errorf("agent: decode tool input: %w", err)
		}
		out := make([]ParsedPersonnelRow, len(payload.Rows))
		for i, r := range payload.Rows {
			out[i] = ParsedPersonnelRow{
				FullName:      r.FullName,
				Rank:          r.Rank,
				Battery:       r.Battery,
				NRICLast5:     r.NRICLast5,
				Extras:        r.Extras,
				SourceSnippet: r.SourceSnippet,
				Confidence:    r.Confidence,
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("agent: response did not include a %s tool call", personnelToolName)
}

// normalizeRows applies cheap normalisations the model occasionally
// misses: trim spaces, uppercase NRIC, uppercase rank.
func normalizeRows(rows []ParsedPersonnelRow) {
	for i := range rows {
		rows[i].FullName = strings.TrimSpace(rows[i].FullName)
		rows[i].Rank = strings.ToUpper(strings.TrimSpace(rows[i].Rank))
		rows[i].Battery = strings.TrimSpace(rows[i].Battery)
		rows[i].NRICLast5 = strings.ToUpper(strings.TrimSpace(rows[i].NRICLast5))
	}
}

// normalizeMediaType picks a content type from the explicit MIME (if
// given) or falls back to the filename extension. Browser-supplied
// MIME types for XLSX/DOCX are often missing or wrong, so the
// extension is a reliable backstop.
func normalizeMediaType(mediaType, filename string) string {
	if mediaType != "" && mediaType != "application/octet-stream" {
		return mediaType
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".xlsx":
		return mediaTypeXLSX
	case ".xlsm":
		return mediaTypeXLSM
	case ".xls":
		return mediaTypeXLS
	case ".docx":
		return mediaTypeDOCX
	case ".doc":
		return mediaTypeDOC
	case ".pdf":
		return mediaTypePDF
	}
	return mediaType
}

// IsValidNRICLast5 mirrors the feature-001 validation rule (4 digits +
// 1 letter). Kept local to the agent package so we don't depend on
// handlers/.
func IsValidNRICLast5(value string) bool {
	if len(value) != 5 {
		return false
	}
	if strings.TrimSpace(value) != value {
		return false
	}
	for i := 0; i < 4; i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	last := value[4]
	return (last >= 'A' && last <= 'Z') || (last >= 'a' && last <= 'z')
}
