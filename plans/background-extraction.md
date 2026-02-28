<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Background Extraction with Processing Indicator

Issue: #490
Depends on: PR #483 (merged)

## Problem

The extraction overlay blocks the entire UI while running. For large documents
or slow LLM models, this can take minutes. Users should be able to background
the extraction and continue working.

## Design

### Data model

**Extraction identity** -- each `extractionLogState` gets a unique `ID uint64`
assigned from an atomic counter (`nextExtractionID`). Messages carry this ID
for routing.

**Model fields:**

```go
extraction     *extractionLogState   // foreground (overlay visible)
bgExtractions  []*extractionLogState // backgrounded (running or done, no overlay)
```

When a new extraction starts and `m.extraction != nil`, the existing foreground
extraction is auto-backgrounded (not cancelled). This is essential for the
multi-upload scenario (#488).

### Message routing

Add `ID uint64` to `extractionProgressMsg`, `extractionLLMChunkMsg`, and
`extractionLLMStartedMsg`. The `waitForExtractProgress` and `waitForLLMChunk`
helpers accept the ID and embed it in the returned message.

A `findExtraction(id uint64) *extractionLogState` helper checks the foreground
extraction first, then scans `bgExtractions`. Handlers use this instead of
`m.extraction` directly.

### Key bindings

| Key      | Context              | Action                                   |
|----------|----------------------|------------------------------------------|
| `ctrl+b` | Extraction overlay   | Background: move to bgExtractions, close |
| `ctrl+b` | Normal/edit mode     | Foreground: bring back most recent bg    |

Add `keyCtrlB = "ctrl+b"` constant in `model.go`.

The extraction overlay hint line gains a `ctrl+b` "background" hint when the
extraction is still running (not `Done`).

### Spinner ticking

The `spinner.TickMsg` handler already runs for `m.extraction`. Extend it to
also tick every non-Done entry in `bgExtractions`, collecting all returned
cmds into the batch.

### Review stack (no auto-accept)

Background extractions are **never** auto-accepted. When a background
extraction completes:

1. **Success** (`!HasError`): `setStatusInfo("Extracted: <filename>")`. The
   extraction stays in `bgExtractions` awaiting user review.
2. **Error** (`HasError`): `setStatusError("Extraction failed: <filename>")`.
   The extraction stays in `bgExtractions` so the user can foreground to
   inspect.

The user must foreground a completed extraction (`ctrl+b`) to review the
results and explicitly accept (`a`) or discard (`esc`).

### Re-open / foreground

`ctrl+b` in normal mode with `len(bgExtractions) > 0`:

- Move `bgExtractions[len-1]` (most recent) to `m.extraction`.
- Set `ex.Visible = true`.
- If `m.extraction` was already non-nil and still running, background it first.

This gives simple stack-like cycling.

### Status bar indicator

When `len(bgExtractions) > 0`, prepend a spinner + count indicator to the
status bar. Distinguish between running and ready-for-review:

- Running: `[spinner] N extracting` (with animated spinner)
- Ready: `N ready` (no spinner, accent styled)

Render before the mode badge in `statusView()`. Style with
`appStyles.AccentText()`.

### Cancel

`cancelExtraction()` continues to cancel the foreground extraction only.
Background extractions can be cancelled by foregrounding (`ctrl+b`) then
pressing `esc`.

Add cleanup to the quit handler to cancel all bg extractions.

### Overlay rendering

No changes to `buildView()` -- it still checks
`m.extraction != nil && m.extraction.Visible`. Background extractions have
`Visible = false` and are never rendered.

## Implementation steps

1. Add `ID uint64` field to `extractionLogState`, `nextExtractionID` atomic
   counter, and `bgExtractions` field to `Model`.
2. Add `keyCtrlB` constant.
3. Add ID to message types and `waitFor*` helpers.
4. Implement `findExtraction(id)` and refactor `handleExtractionProgress`,
   `handleExtractionLLMChunk`, `handleExtractionLLMStarted` to use it.
5. Implement `backgroundExtraction()` -- moves foreground to bg list.
6. Implement `foregroundExtraction()` -- moves most recent bg to foreground.
7. Add `ctrl+b` handling in extraction overlay keys.
8. Add `ctrl+b` handling in normal/edit mode dispatch.
9. Auto-background existing foreground when `startExtractionOverlay` is called.
10. Notify on background completion (status messages, NO auto-accept).
11. Extend spinner ticking to bg extractions.
12. Status bar indicator rendering.
13. Cleanup on quit.
14. Hint line: add `ctrl+b` "background" hint in overlay when `!Done`.
15. Tests: user-flow tests for background, foreground, completion notify, and
    multiple concurrent extractions.
