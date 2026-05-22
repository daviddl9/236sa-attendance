package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// generateTestXLSX builds an .xlsx in memory from the given rows so
// tests do not need a fixture file on disk.
func generateTestXLSX(t *testing.T, rows [][]string) []byte {
	t.Helper()
	xl := excelize.NewFile()
	defer xl.Close()
	sheet := xl.GetSheetName(0)
	for r, row := range rows {
		for c, cell := range row {
			axis, err := excelize.CoordinatesToCellName(c+1, r+1)
			if err != nil {
				t.Fatalf("coords: %v", err)
			}
			if err := xl.SetCellValue(sheet, axis, cell); err != nil {
				t.Fatalf("set cell: %v", err)
			}
		}
	}
	var buf bytes.Buffer
	if err := xl.Write(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	return buf.Bytes()
}

// fakeClient returns canned responses or canned errors so the parser
// can be tested without touching the network.
type fakeClient struct {
	resp *MessageResponse
	err  error

	gotRequest MessageRequest
}

func (f *fakeClient) CreateMessage(_ context.Context, req MessageRequest) (*MessageResponse, error) {
	f.gotRequest = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

// toolUseResponse builds a MessageResponse that mimics a real
// Anthropic response containing a tool_use block with the given JSON
// input.
func toolUseResponse(t *testing.T, input string) *MessageResponse {
	t.Helper()
	return &MessageResponse{
		ID:   "msg_test",
		Type: "message",
		Role: "assistant",
		Content: []ContentBlock{
			{
				Type:  "tool_use",
				ID:    "toolu_test",
				Name:  personnelToolName,
				Input: json.RawMessage(input),
			},
		},
		Model:      "claude-sonnet-4-6",
		StopReason: "tool_use",
	}
}

// minimalXLSX is a small valid .xlsx file containing one personnel
// row. Built once at startup by encoding from raw bytes is fragile, so
// we generate it inline at test time via excelize when needed.
func minimalXLSXBytes(t *testing.T) []byte {
	t.Helper()
	// Two rows: header + one personnel record. Doesn't need to be
	// large — the parser only reads the bytes to send to the agent;
	// the fake client returns the golden response regardless of input.
	return generateTestXLSX(t, [][]string{
		{"Full Name", "Rank", "Battery", "NRIC Last 5"},
		{"JOHN TAN", "3SG", "Alpha", "1234A"},
	})
}

func TestParseDocument_XLSX_GoldenFixture(t *testing.T) {
	fake := &fakeClient{
		resp: toolUseResponse(t, `{
			"rows": [
				{
					"full_name": "John Tan",
					"rank": "3sg",
					"battery": "Alpha",
					"nric_last5": "1234a",
					"extras": {"callup_serial": "001"},
					"source_snippet": "JOHN TAN 3SG Alpha 1234A",
					"confidence": 0.95
				},
				{
					"full_name": "Mary Lim",
					"rank": "CPT",
					"battery": "HQ",
					"nric_last5": "5678B",
					"source_snippet": "MARY LIM CPT HQ 5678B",
					"confidence": 1.0
				}
			]
		}`),
	}
	parser := NewParser(fake)

	rows, err := parser.ParseDocument(context.Background(), "roster.xlsx", minimalXLSXBytes(t), mediaTypeXLSX)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	// Normalisation: NRIC uppercased, rank uppercased, FullName trimmed.
	if rows[0].FullName != "John Tan" || rows[0].NRICLast5 != "1234A" || rows[0].Rank != "3SG" {
		t.Errorf("row[0] not normalised: %+v", rows[0])
	}
	if rows[0].Extras["callup_serial"] != "001" {
		t.Errorf("row[0] extras lost: %+v", rows[0].Extras)
	}
	if rows[1].FullName != "Mary Lim" || rows[1].NRICLast5 != "5678B" {
		t.Errorf("row[1] wrong: %+v", rows[1])
	}

	// Verify the request the parser sent: must be a text block (XLSX
	// rendered to markdown), must include the tool, must force the
	// tool choice.
	if len(fake.gotRequest.Messages) != 1 {
		t.Fatalf("expected one user message, got %d", len(fake.gotRequest.Messages))
	}
	if got := fake.gotRequest.Messages[0].Content[0].Type; got != "text" {
		t.Errorf("XLSX should be sent as text content, got %q", got)
	}
	if !strings.Contains(fake.gotRequest.Messages[0].Content[0].Text, "JOHN TAN") {
		t.Errorf("rendered markdown should include the personnel row, got: %q", fake.gotRequest.Messages[0].Content[0].Text)
	}
	if fake.gotRequest.ToolChoice == nil || fake.gotRequest.ToolChoice.Name != personnelToolName {
		t.Errorf("tool_choice must force %s, got %+v", personnelToolName, fake.gotRequest.ToolChoice)
	}
}

func TestParseDocument_PDF_UsesDocumentBlock(t *testing.T) {
	fake := &fakeClient{
		resp: toolUseResponse(t, `{"rows": [
			{"full_name": "Alice", "nric_last5": "9999Z", "confidence": 0.9}
		]}`),
	}
	parser := NewParser(fake)

	_, err := parser.ParseDocument(context.Background(), "roster.pdf", []byte("%PDF-1.4 dummy"), mediaTypePDF)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}

	content := fake.gotRequest.Messages[0].Content
	if len(content) != 2 {
		t.Fatalf("expected document+text content blocks, got %d", len(content))
	}
	if content[0].Type != "document" {
		t.Errorf("first block must be document, got %q", content[0].Type)
	}
	if content[0].Source == nil || content[0].Source.MediaType != mediaTypePDF {
		t.Errorf("document source incorrect: %+v", content[0].Source)
	}
}

func TestParseDocument_ErrorMapping(t *testing.T) {
	xlsx := minimalXLSXBytes(t)

	tests := []struct {
		name    string
		client  *fakeClient
		want    error
		wantMsg string // substring match when want is nil
	}{
		{
			name:   "API timeout",
			client: &fakeClient{err: ErrAPITimeout},
			want:   ErrAPITimeout,
		},
		{
			name:   "invalid API key",
			client: &fakeClient{err: ErrInvalidAPIKey},
			want:   ErrInvalidAPIKey,
		},
		{
			name:   "no rows returned",
			client: &fakeClient{resp: toolUseResponse(t, `{"rows": []}`)},
			want:   ErrNoRowsDetected,
		},
		{
			name: "response missing tool_use",
			client: &fakeClient{resp: &MessageResponse{
				Content: []ContentBlock{{Type: "text", Text: "hi"}},
			}},
			wantMsg: "did not include a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.client)
			_, err := parser.ParseDocument(context.Background(), "x.xlsx", xlsx, mediaTypeXLSX)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("want %v, got %v", tt.want, err)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error %q must contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestParseDocument_TooLarge(t *testing.T) {
	parser := NewParser(&fakeClient{})
	parser.maxBytes = 16
	_, err := parser.ParseDocument(context.Background(), "big.pdf", make([]byte, 32), mediaTypePDF)
	if !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("want ErrDocumentTooLarge, got %v", err)
	}
}

func TestParseDocument_EmptyContents(t *testing.T) {
	parser := NewParser(&fakeClient{})
	_, err := parser.ParseDocument(context.Background(), "empty.xlsx", nil, mediaTypeXLSX)
	if !errors.Is(err, ErrNoRowsDetected) {
		t.Fatalf("want ErrNoRowsDetected, got %v", err)
	}
}

func TestParseDocument_UnsupportedMediaType(t *testing.T) {
	parser := NewParser(&fakeClient{})
	_, err := parser.ParseDocument(context.Background(), "x.bin", []byte("anything"), "application/octet-stream")
	if err == nil || !strings.Contains(err.Error(), "unsupported media type") {
		t.Fatalf("want unsupported-media-type error, got %v", err)
	}
}

func TestNormalizeMediaType_FallbackByExtension(t *testing.T) {
	tests := []struct {
		filename string
		media    string
		want     string
	}{
		{"r.xlsx", "", mediaTypeXLSX},
		{"r.XLSX", "application/octet-stream", mediaTypeXLSX},
		{"r.docx", "", mediaTypeDOCX},
		{"r.pdf", "", mediaTypePDF},
		{"r.xlsx", mediaTypePDF, mediaTypePDF}, // explicit MIME wins
	}
	for _, tt := range tests {
		got := normalizeMediaType(tt.media, tt.filename)
		if got != tt.want {
			t.Errorf("normalizeMediaType(%q, %q) = %q, want %q", tt.media, tt.filename, got, tt.want)
		}
	}
}

func TestIsValidNRICLast5(t *testing.T) {
	good := []string{"1234A", "0000Z", "9999a"}
	bad := []string{"", "1234", "12345", "12A4B", "abcde", "1234A ", " 1234A"}
	for _, v := range good {
		if !IsValidNRICLast5(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	for _, v := range bad {
		if IsValidNRICLast5(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}
