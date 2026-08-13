package telegram

import (
	"errors"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

const qrImageSize = 256

// QRPNG encodes a Telegram attendance link as an in-memory PNG image.
func QRPNG(link string) ([]byte, error) {
	link = strings.TrimSpace(link)
	if link == "" {
		return nil, errors.New("telegram QR link is required")
	}
	return qrcode.Encode(link, qrcode.Medium, qrImageSize)
}
