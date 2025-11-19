package stt

import (
	"context"
	"errors"
	"io"
)

// MockProvider is a mock implementation for testing and development
// In production, this should be replaced with a real provider (Google Cloud Speech, AWS Transcribe, etc.)
type MockProvider struct {
	supportedLanguages []string
	supportedFormats   []string
}

// NewMockProvider creates a new mock STT provider
func NewMockProvider() *MockProvider {
	return &MockProvider{
		supportedLanguages: []string{"ko-KR", "en-US", "ja-JP"},
		supportedFormats:   []string{"wav", "mp3", "m4a", "webm"},
	}
}

// Transcribe returns a mock transcription result
func (m *MockProvider) Transcribe(ctx context.Context, audio io.Reader, opts TranscriptionOptions) (*TranscriptionResult, error) {
	// In a real implementation, this would:
	// 1. Validate audio format
	// 2. Call the STT API (Google Cloud Speech, AWS Transcribe, etc.)
	// 3. Return the transcribed text

	if audio == nil {
		return nil, errors.New("audio input is required")
	}

	// Check language support
	if opts.Language != "" && !m.SupportsLanguage(opts.Language) {
		return nil, errors.New("unsupported language: " + opts.Language)
	}

	// Return mock result for development
	return &TranscriptionResult{
		Text:       "This is a mock transcription. Replace MockProvider with a real implementation.",
		Confidence: 0.95,
		Language:   opts.Language,
		Duration:   2.5,
	}, nil
}

// SupportsLanguage checks if the language is supported
func (m *MockProvider) SupportsLanguage(language string) bool {
	for _, lang := range m.supportedLanguages {
		if lang == language {
			return true
		}
	}
	return false
}

// GetSupportedFormats returns supported audio formats
func (m *MockProvider) GetSupportedFormats() []string {
	return m.supportedFormats
}
