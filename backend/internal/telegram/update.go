package telegram

import (
	"encoding/json"
	"fmt"
	"io"
)

// Update is the subset of a Telegram update needed by this application.
// Pointers distinguish an absent update kind from an update containing zero
// values, and make missing fields safe to handle.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from,omitempty"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *User    `json:"from,omitempty"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

// DecodeUpdate decodes one Telegram webhook payload. Unknown fields are
// ignored because Telegram may add fields that this task does not use.
func DecodeUpdate(reader io.Reader) (Update, error) {
	var update Update
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&update); err != nil {
		return Update{}, fmt.Errorf("decode Telegram update: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Update{}, fmt.Errorf("decode Telegram update: multiple JSON values")
		}
		return Update{}, fmt.Errorf("decode Telegram update: %w", err)
	}
	return update, nil
}
