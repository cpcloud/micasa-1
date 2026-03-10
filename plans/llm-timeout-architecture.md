<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# LLM Timeout Architecture Refactor

Issue: #732

## Problem

The current timeout architecture conflates two concerns:

1. `llm.timeout` (default 5m) is the HTTP client `ResponseTimeout` for ALL
   requests -- way too long for quick ops, and it conflates "how long to wait
   for the server to respond at all" with "how long a streaming response is
   allowed to take."

2. `QuickOpTimeout` (30s) is hardcoded in `internal/llm/client.go` for ping
   and model listing. Not configurable, completely ignores `llm.timeout`.

3. `extraction.llm_timeout` (default 5m) is a separate context deadline for
   extraction inference, but `llm.timeout` also applies as the HTTP-level
   timeout on the same request (double-layered).

4. `llm.chat.timeout` / `llm.extraction.timeout` override the HTTP client
   timeout -- yet another layer.

5. Chat has NO inference context deadline at all (only the HTTP client timeout).

## Design

Two distinct timeout concerns with clear ownership:

### Quick-op timeout (constant, 30s)

For fast LLM server operations: ping, model listing, auto-detect. Shared
across both pipelines. Stays as `QuickOpTimeout` constant -- not
user-configurable since 30s is universally appropriate for these operations.

### Per-pipeline inference timeout (context deadline)

How long chat or extraction inference is allowed to take. Each pipeline has
its own config key, inheriting from `llm.timeout` when not set:

- `llm.timeout` (default 5m) -- base inference timeout, inherited by both
- `llm.chat.timeout` -- overrides for chat (inherits `llm.timeout`)
- `llm.extraction.timeout` -- overrides for extraction (inherits `llm.timeout`,
  replaces deprecated `extraction.llm_timeout`)

Chat streaming now enforces a context deadline using this timeout
(previously had no deadline at all).

### HTTP client timeout (derived)

`max(QuickOpTimeout, inferenceTimeout)` -- ensures the HTTP client never
kills a request before the context deadline does. Derived inside `NewClient`,
not directly configurable.

## Changes

### Config (`internal/config/config.go`)

- `llm.timeout` doc: clarified as base inference timeout with per-pipeline
  overrides
- `ResolvedLLM.Timeout`: now the inference context deadline (was HTTP timeout)
- `resolvePipeline`: per-pipeline timeout coalesces with `llm.timeout`, then
  defaults to `DefaultLLMTimeout`
- Deprecate `extraction.llm_timeout` -> migrate to `llm.extraction.timeout`
- Deprecate `MICASA_EXTRACTION_LLM_TIMEOUT` env var
- `extractionConfig.LLMInferenceTimeout` removed (Timeout IS the inference
  timeout now)
- `SetExtraction` no longer takes a separate `llmInferenceTimeout` param

### LLM client (`internal/llm/client.go`)

- HTTP client timeout = `max(timeout, QuickOpTimeout)` in `NewClient`
- `QuickOpTimeout` constant stays (not configurable)
- Timeout error message updated to reference all three timeout config keys

### App layer (`internal/app/`)

- Chat streaming: `WithCancel` -> `WithTimeout` using `llmConfig.Timeout`
- Extraction: uses `extractionTimeout` directly (was separate
  `llmInferenceTimeout`)
- Error message in extraction timeout: references `llm.extraction.timeout`

### Main (`cmd/micasa/main.go`)

- Remove separate `cfg.Extraction.LLMTimeoutDuration()` pass-through

## Migration

| Old | New | Action |
|-----|-----|--------|
| `extraction.llm_timeout` | `llm.extraction.timeout` | TOML key migration + deprecation warning |
| `MICASA_EXTRACTION_LLM_TIMEOUT` | `MICASA_LLM_EXTRACTION_TIMEOUT` | Env var migration + deprecation warning |
