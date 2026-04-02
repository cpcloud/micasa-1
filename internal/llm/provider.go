// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package llm

import (
	"context"
	"time"
)

// Base contains model management methods shared by all LLM providers.
type Base interface {
	Model() string
	SetModel(model string)
	SetEffort(level string)
	ProviderName() string
	BaseURL() string
	Timeout() time.Duration
	IsLocalServer() bool
	SupportsModelListing() bool
	Ping(ctx context.Context) error
	ListModels(ctx context.Context) ([]string, error)
}

// ChatProvider streams chat completions (NL->SQL, summaries).
type ChatProvider interface {
	Base
	ChatStream(
		ctx context.Context,
		messages []Message,
	) (<-chan StreamChunk, error)
}

// ExtractionProvider streams structured extraction output constrained
// by a JSON schema.
type ExtractionProvider interface {
	Base
	ExtractStream(
		ctx context.Context,
		messages []Message,
		schema map[string]any,
	) (<-chan StreamChunk, error)
}

var (
	_ ChatProvider       = (*Client)(nil)
	_ ExtractionProvider = (*Client)(nil)
)
