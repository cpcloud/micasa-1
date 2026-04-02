// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package claudecli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/micasa-dev/micasa/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHelperProcess is re-executed by tests as a mock claude binary.
func TestHelperProcess(_ *testing.T) {
	if os.Getenv("CLAUDE_MOCK_PROCESS") != "1" {
		return
	}

	mode := os.Getenv("CLAUDE_MOCK_MODE")
	switch mode {
	case "stream_schema":
		for _, ev := range []map[string]any{
			{"type": "system", "subtype": "init"},
			{"type": "stream_event", "event": map[string]any{
				"type": "content_block_delta",
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": `{"greeting":`,
				},
			}},
			{"type": "stream_event", "event": map[string]any{
				"type": "content_block_delta",
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": ` "hello"}`,
				},
			}},
			{"type": "stream_event", "event": map[string]any{
				"type": "message_stop",
			}},
			{
				"type": "result", "subtype": "success",
				"result": "", "stop_reason": "end_turn",
			},
		} {
			writeJSON(ev)
		}

	case "stream_error":
		writeJSON(map[string]any{
			"type": "stream_event", "event": map[string]any{
				"type": "content_block_delta",
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": `{"par`,
				},
			},
		})
		fmt.Fprintln(os.Stderr, "Error: connection lost")
		os.Exit(1)

	case "echo_args":
		// Echo captured args as a stream event so tests can verify flags.
		writeJSON(map[string]any{
			"type": "stream_event", "event": map[string]any{
				"type": "content_block_delta",
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": os.Getenv("CLAUDE_MOCK_CAPTURED_ARGS"),
				},
			},
		})
		writeJSON(map[string]any{
			"type": "stream_event", "event": map[string]any{
				"type": "message_stop",
			},
		})

	case "realistic_extraction":
		// Simulates a real claude extraction: system events, thinking,
		// text_delta noise (model's prose), then input_json_delta
		// (structured output), then second turn (should be ignored).
		for _, ev := range []map[string]any{
			{"type": "system", "subtype": "init"},
			{"type": "system", "subtype": "hook_started"},
			{"type": "system", "subtype": "hook_response"},
			// Thinking block
			{"type": "stream_event", "event": map[string]any{
				"type":          "content_block_start",
				"content_block": map[string]any{"type": "thinking"},
			}},
			{"type": "stream_event", "event": map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]any{"type": "thinking_delta", "thinking": "analyzing document..."},
			}},
			{"type": "stream_event", "event": map[string]any{
				"type": "content_block_stop",
			}},
			// Text response (model's prose -- should be filtered)
			{"type": "stream_event", "event": map[string]any{
				"type":          "content_block_start",
				"content_block": map[string]any{"type": "text"},
			}},
			{"type": "stream_event", "event": map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]any{"type": "text_delta", "text": "Here's what I found:"},
			}},
			{"type": "stream_event", "event": map[string]any{
				"type": "content_block_stop",
			}},
			// Tool use: structured JSON output
			{"type": "stream_event", "event": map[string]any{
				"type":          "content_block_start",
				"content_block": map[string]any{"type": "tool_use", "name": "StructuredOutput"},
			}},
			{"type": "stream_event", "event": map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]any{"type": "input_json_delta", "partial_json": `{"operations":[`},
			}},
			{"type": "stream_event", "event": map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]any{"type": "input_json_delta", "partial_json": `{"action":"create","table":"vendors",`},
			}},
			{"type": "stream_event", "event": map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]any{"type": "input_json_delta", "partial_json": `"data":{"name":"Acme Plumbing","phone":"555-1234"}}`},
			}},
			{"type": "stream_event", "event": map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]any{"type": "input_json_delta", "partial_json": `]}`},
			}},
			{"type": "stream_event", "event": map[string]any{
				"type": "content_block_stop",
			}},
			{"type": "assistant"},
			// First turn done
			{"type": "stream_event", "event": map[string]any{
				"type": "message_stop",
			}},
			// Second turn (tool result processing -- should be ignored).
			// Uses input_json_delta so a regression that keeps reading
			// would corrupt the accumulated JSON.
			{"type": "stream_event", "event": map[string]any{
				"type": "message_start",
			}},
			{"type": "stream_event", "event": map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]any{"type": "input_json_delta", "partial_json": `,"SECOND_TURN_GARBAGE":true`},
			}},
			{"type": "stream_event", "event": map[string]any{
				"type": "message_stop",
			}},
			{"type": "result", "subtype": "success", "result": "", "stop_reason": "end_turn"},
		} {
			writeJSON(ev)
		}

	case "malformed_json":
		// Write syntactically invalid JSON that breaks json.Decoder.Decode.
		_, _ = fmt.Fprintln(os.Stdout, `{this is not valid json at all`)

	default:
		fmt.Fprintf(os.Stderr, "unknown mock mode: %s\n", mode)
		os.Exit(2)
	}

	os.Exit(0)
}

func writeJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) //nolint:errchkjson // test helper, errors are not actionable
}

func mockCmdFactory(mode string) cmdFactory {
	return func(ctx context.Context, args ...string) *exec.Cmd {
		cmd := exec.CommandContext( //nolint:gosec // test mock binary, args are test-controlled
			ctx,
			os.Args[0],
			"-test.run=^TestHelperProcess$",
		)
		cmd.Env = append(os.Environ(),
			"CLAUDE_MOCK_PROCESS=1",
			"CLAUDE_MOCK_MODE="+mode,
			"CLAUDE_MOCK_CAPTURED_ARGS="+strings.Join(args, " "),
		)
		return cmd
	}
}

func newMockClient(t *testing.T, mode string) *Client {
	t.Helper()
	c, err := NewClient("test-model", 5*time.Second,
		withCmdFactory(mockCmdFactory(mode)),
	)
	require.NoError(t, err)
	return c
}

func drainStream(ch <-chan llm.StreamChunk) (string, error) {
	var b strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			return b.String(), chunk.Err
		}
		b.WriteString(chunk.Content)
	}
	return b.String(), nil
}

var testSchema = map[string]any{"type": "object"}

// --- NewClient tests ---

func TestNewClient(t *testing.T) {
	t.Parallel()

	t.Run("with valid binary path", func(t *testing.T) {
		t.Parallel()
		c, err := NewClient(
			"test-model", 5*time.Second, WithBinPath(os.Args[0]),
		)
		require.NoError(t, err)
		assert.Equal(t, "test-model", c.Model())
		assert.Equal(t, "claude-cli", c.ProviderName())
	})

	t.Run("zero timeout rejected", func(t *testing.T) {
		t.Parallel()
		_, err := NewClient(
			"m", 0, withCmdFactory(mockCmdFactory("stream_schema")),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timeout")
	})

	t.Run("with nonexistent path fails", func(t *testing.T) {
		t.Parallel()
		_, err := NewClient(
			"m", time.Second, WithBinPath("/nonexistent/claude"),
		)
		require.Error(t, err)
	})

	t.Run("trivial methods", func(t *testing.T) {
		t.Parallel()
		c := newMockClient(t, "stream_schema")

		assert.Empty(t, c.BaseURL())
		assert.Equal(
			t, llm.QuickOpTimeout, c.Timeout(),
			"Timeout() returns QuickOpTimeout regardless of construction timeout",
		)
		assert.False(t, c.IsLocalServer())
		assert.False(t, c.SupportsModelListing())
		require.NoError(t, c.Ping(context.Background()))

		_, err := c.ListModels(context.Background())
		require.Error(t, err)

		c.SetModel("new-model")
		assert.Equal(t, "new-model", c.Model())

		c.SetEffort("high")
	})
}

// --- ExtractStream tests ---

func TestExtractStream(t *testing.T) {
	t.Parallel()

	t.Run("streams input_json_delta fragments", func(t *testing.T) {
		t.Parallel()
		c := newMockClient(t, "stream_schema")
		ch, err := c.ExtractStream(
			context.Background(),
			[]llm.Message{{Role: "user", Content: "extract this"}},
			testSchema,
		)
		require.NoError(t, err)

		content, err := drainStream(ch)
		require.NoError(t, err)
		assert.JSONEq(t, `{"greeting": "hello"}`, content)
	})

	t.Run("surfaces stderr on non-zero exit", func(t *testing.T) {
		t.Parallel()
		c := newMockClient(t, "stream_error")
		ch, err := c.ExtractStream(
			context.Background(),
			[]llm.Message{{Role: "user", Content: "extract"}},
			testSchema,
		)
		require.NoError(t, err)

		_, err = drainStream(ch)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection lost")
	})

	t.Run("rejects multi-turn messages", func(t *testing.T) {
		t.Parallel()
		c := newMockClient(t, "stream_schema")
		_, err := c.ExtractStream(
			context.Background(),
			[]llm.Message{
				{Role: "system", Content: "sys"},
				{Role: "user", Content: "a"},
				{Role: "assistant", Content: "b"},
			},
			testSchema,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "single-turn")
	})

	t.Run("rejects empty messages", func(t *testing.T) {
		t.Parallel()
		c := newMockClient(t, "stream_schema")
		_, err := c.ExtractStream(
			context.Background(), nil, testSchema,
		)
		require.Error(t, err)
	})

	t.Run("passes model and system-prompt flags", func(t *testing.T) {
		t.Parallel()
		c := newMockClient(t, "echo_args")
		ch, err := c.ExtractStream(
			context.Background(),
			[]llm.Message{
				{Role: "system", Content: "be helpful"},
				{Role: "user", Content: "hello"},
			},
			testSchema,
		)
		require.NoError(t, err)
		args, err := drainStream(ch)
		require.NoError(t, err)
		assert.Contains(t, args, "--model")
		assert.Contains(t, args, "test-model")
		assert.Contains(t, args, "--system-prompt")
		assert.Contains(t, args, "--json-schema")
	})

	t.Run("effort mapping", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			effort   string
			contains string
			absent   bool
		}{
			{"", "", true},
			{"none", "", true},
			{"auto", "", true},
			{"low", "--effort low", false},
			{"medium", "--effort medium", false},
			{"high", "--effort high", false},
		}

		for _, tt := range tests {
			t.Run(tt.effort, func(t *testing.T) {
				t.Parallel()
				c := newMockClient(t, "echo_args")
				c.SetEffort(tt.effort)
				ch, err := c.ExtractStream(
					context.Background(),
					[]llm.Message{{Role: "user", Content: "hi"}},
					testSchema,
				)
				require.NoError(t, err)
				args, err := drainStream(ch)
				require.NoError(t, err)
				if tt.absent {
					assert.NotContains(t, args, "--effort")
				} else {
					assert.Contains(t, args, tt.contains)
				}
			})
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c := newMockClient(t, "stream_schema")
		ch, err := c.ExtractStream(
			ctx,
			[]llm.Message{{Role: "user", Content: "hello"}},
			testSchema,
		)
		if err != nil {
			return // error at start is fine
		}
		_, err = drainStream(ch)
		assert.Error(t, err)
	})

	t.Run("realistic extraction filters noise and stops at first turn", func(t *testing.T) {
		t.Parallel()
		// Simulates a real claude CLI extraction stream: system hooks,
		// thinking blocks, text_delta prose (filtered), input_json_delta
		// (forwarded), then a second turn that should be ignored.
		c := newMockClient(t, "realistic_extraction")
		ch, err := c.ExtractStream(
			context.Background(),
			[]llm.Message{
				{Role: "system", Content: "extract entities"},
				{Role: "user", Content: "Acme Plumbing 555-1234"},
			},
			testSchema,
		)
		require.NoError(t, err)

		content, err := drainStream(ch)
		require.NoError(t, err)

		// Should contain ONLY the JSON operations, not the prose or
		// second turn garbage.
		assert.NotContains(t, content, "Here's what I found")
		assert.NotContains(t, content, "SECOND_TURN_GARBAGE")
		assert.NotContains(t, content, "analyzing document")
		assert.Contains(t, content, "Acme Plumbing")
		assert.Contains(t, content, "555-1234")

		// Should be valid JSON.
		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(content), &parsed))
		ops, ok := parsed["operations"].([]any)
		require.True(t, ok)
		require.Len(t, ops, 1)
	})

	t.Run("malformed NDJSON surfaces decode error not cancellation", func(t *testing.T) {
		t.Parallel()
		c := newMockClient(t, "malformed_json")
		ch, err := c.ExtractStream(
			context.Background(),
			[]llm.Message{{Role: "user", Content: "extract"}},
			testSchema,
		)
		require.NoError(t, err)

		_, err = drainStream(ch)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "malformed stream")
		assert.NotContains(t, err.Error(), "context canceled")
	})
}
