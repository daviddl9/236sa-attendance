package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/sashabaranov/go-openai"
)

type ChatHandler struct {
	client *openai.Client
}

func NewChatHandler() *ChatHandler {
	apiKey := os.Getenv("OPENAI_API_KEY")
	var client *openai.Client
	if apiKey != "" {
		client = openai.NewClient(apiKey)
	}
	return &ChatHandler{
		client: client,
	}
}

type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
	Model    string        `json:"model"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (h *ChatHandler) StreamChat(w http.ResponseWriter, r *http.Request) {
	if h.client == nil {
		http.Error(w, "OpenAI API key not configured", http.StatusServiceUnavailable)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Default model if not specified
	if req.Model == "" {
		req.Model = openai.GPT4
	}

	// Convert messages
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	ctx := context.Background()
	stream, err := h.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   true,
	})
	if err != nil {
		http.Error(w, "Failed to create chat stream: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = stream.Close() }()

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Stream the response
	for {
		response, err := stream.Recv()
		if err == io.EOF {
			// Send done message
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			flusher.Flush()
			break
		}
		if err != nil {
			// Send error and close
			_, _ = w.Write([]byte("event: error\ndata: " + err.Error() + "\n\n"))
			flusher.Flush()
			break
		}

		// Send the chunk
		if len(response.Choices) > 0 {
			data, _ := json.Marshal(response)
			_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
			flusher.Flush()
		}
	}
}
