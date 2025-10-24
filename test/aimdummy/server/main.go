package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

func printReqDebug(r *http.Request) {
	reqDump, err := httputil.DumpRequest(r, true)
	if err != nil {
		fmt.Println("Failed to debug request", err)
	}
	fmt.Printf("REQUEST:\n%s\n", string(reqDump))
}

// OpenAI API Response Structures
type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func main() {
	fmt.Println("Starting OpenAI API Mock Server on port 8000")

	// Health check endpoint
	http.HandleFunc("/health", healthHandler)

	// OpenAI API endpoints
	http.HandleFunc("/v1/models", modelsHandler)
	http.HandleFunc("/v1/chat/completions", chatCompletionsHandler)

	// Root endpoint
	http.HandleFunc("/", rootHandler)

	s := &http.Server{
		Addr:           ":8000",
		Handler:        nil,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	log.Fatal(s.ListenAndServe())
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "healthy"}`)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"message": "OpenAI API Mock Server", "version": "1.0.0"}`)
}

func modelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", "method_not_allowed", "", http.StatusMethodNotAllowed)
		return
	}

	// Mock available models
	models := ModelsResponse{
		Object: "list",
		Data: []Model{
			{
				ID:      "gpt-3.5-turbo",
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "openai",
			},
			{
				ID:      "gpt-4",
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "openai",
			},
			{
				ID:      "qwen2.5-0.5b-instruct",
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "aim-mock",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(models)
}

func chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("chat completion called")
	printReqDebug(r)

	if r.Method != http.MethodPost {
		writeError(w, "Method not allowed", "method_not_allowed", "", http.StatusMethodNotAllowed)
		return
	}

	// Check Authorization header
	/*
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			writeError(w, "Missing or invalid authorization header", "invalid_request_error", "invalid_api_key", http.StatusUnauthorized)
			return
		}
	*/

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid JSON in request body", "invalid_request_error", "invalid_json", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Model == "" {
		writeError(w, "Missing required field: model", "invalid_request_error", "missing_field", http.StatusBadRequest)
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, "Missing required field: messages", "invalid_request_error", "missing_field", http.StatusBadRequest)
		return
	}

	// Generate mock response based on the last user message
	lastMessage := req.Messages[len(req.Messages)-1]
	responseContent := generateMockResponse(lastMessage.Content)

	response := ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: responseContent,
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     estimateTokens(lastMessage.Content),
			CompletionTokens: estimateTokens(responseContent),
			TotalTokens:      estimateTokens(lastMessage.Content) + estimateTokens(responseContent),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func generateMockResponse(userMessage string) string {
	// Simple mock responses based on keywords
	lowerMsg := strings.ToLower(userMessage)

	if strings.Contains(lowerMsg, "hello") || strings.Contains(lowerMsg, "hi") {
		return "Hello! I'm a mock OpenAI API server. How can I help you today?"
	}

	if strings.Contains(lowerMsg, "weather") {
		return "I'm a mock server, so I can't provide real weather data. But I can tell you it's always sunny in the world of mocks!"
	}

	if strings.Contains(lowerMsg, "code") || strings.Contains(lowerMsg, "programming") {
		return "Here's a simple example:\n\n```python\nprint('Hello from mock OpenAI API!')\n```\n\nThis is a mock response for programming-related queries."
	}

	if strings.Contains(lowerMsg, "explain") || strings.Contains(lowerMsg, "what is") {
		return "This is a mock explanation. In a real scenario, I would provide detailed information about your topic. Since this is a mock server, I'm giving you this generic response instead."
	}

	// Default response
	return fmt.Sprintf("Thank you for your message: '%s'. This is a mock response from the OpenAI API mock server. In a real implementation, this would be a sophisticated AI-generated response.", userMessage)
}

func estimateTokens(text string) int {
	// Simple token estimation (roughly 4 characters per token)
	return len(text) / 4
}

func writeError(w http.ResponseWriter, message, errorType, code string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := ErrorResponse{
		Error: ErrorDetail{
			Message: message,
			Type:    errorType,
			Code:    code,
		},
	}

	json.NewEncoder(w).Encode(errorResp)
}
