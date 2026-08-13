package telegram

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestQRPNGProducesValidPNG(t *testing.T) {
	data, err := QRPNG("https://attendance.example/start")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("QRPNG returned no bytes")
	}
	if !bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		t.Fatalf("PNG signature = %v", data[:8])
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("decode QR PNG: %v", err)
	}
}

func TestQRPNGRejectsEmptyLink(t *testing.T) {
	for _, link := range []string{"", " ", "\t\n"} {
		data, err := QRPNG(link)
		if err == nil {
			t.Fatalf("QRPNG(%q) succeeded with %d bytes", link, len(data))
		}
		if strings.Contains(err.Error(), link) && strings.TrimSpace(link) != "" {
			t.Fatalf("error unexpectedly contains link %q: %v", link, err)
		}
	}
}
