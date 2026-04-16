<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Status command: colored table output

## Goal

Replace plain `text/tabwriter` output in `micasa status` with styled
`lipgloss/table` tables using the Wong colorblind-safe palette.

## Design

### Rendering

Replace `writeOverdueText`, `writeUpcomingText`, `writeIncidentsText`,
`writeProjectsText` with lipgloss/table equivalents.

- **Section headers**: Rendered as standalone styled strings (not part
  of the table) via `lipgloss.Style.Render()`. Bold blue accent, no
  `===` wrapping. Printed with `fmt.Fprintln` before each table.
- **Table headers**: Bold, dimmed text via `StyleFunc` returning a
  bold+dim style when `row == table.HeaderRow`.
- **Overdue days column**: Vermillion/danger color on the days cell.
- **Incident severity**: Color-coded via `StyleFunc` — urgent=vermillion,
  soon=orange, whenever=muted.
- **Project status**: delayed=vermillion, underway=green.
- **Borders**: `lipgloss.RoundedBorder()`, border color from Wong
  palette border pair.
- **Light/dark**: Detect via `lipgloss.HasDarkBackground(os.Stdin,
  os.Stderr)` in `runStatus`, passed as `isDark bool` to rendering
  functions. Same pattern as `main.go:185`.

### CLI styles

Add a `cliStyles` struct to `theme.go` with styles needed by status
output: section header, table header, danger, warning, success, muted,
border. Built from Wong adaptive pairs resolved with an `isDark` flag.
Parallels `internal/app/styles.go` but stays in `cmd/micasa` to avoid
importing the full TUI package.

Add `isDark bool` to `statusOpts`. The cobra handler sets it via
`lipgloss.HasDarkBackground(os.Stdin, os.Stderr)` before calling
`runStatus`. Tests leave it at the zero value (false) — doesn't
matter since content assertions use `ansi.Strip()`. This avoids
changing `runStatus`'s signature (which 19 test call sites use).

### Testing

`table.String()` always renders with full ANSI styling — it has no
color profile parameter. Tests use `ansi.Strip()` from
`charmbracelet/x/ansi` (already a dependency) to strip all ANSI
escape sequences before content assertions.

- All content tests: render normally, `ansi.Strip()` the output,
  assert on plain text content.
- One smoke test: render normally, verify output contains `\x1b[`
  (confirms styling is present).

### `isDark` in tests

Tests use default `statusOpts{}`  which has `isDark: false`. Content
assertions strip ANSI so the value doesn't matter. The smoke test
explicitly sets `isDark: true` to confirm styles produce ANSI output.

## Files changed

- `cmd/micasa/status.go` — replace tabwriter with lipgloss/table,
  add `isDark` field to `statusOpts`, plumb through to section helpers
- `cmd/micasa/theme.go` — add `cliStyles` struct and constructor
- `cmd/micasa/status_test.go` — use `ansi.Strip()` for content
  assertions, add ANSI smoke test
