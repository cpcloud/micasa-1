<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Fix: drilldown count text not visible when overlay dims background

Issue: micasa-dev/micasa#848

## Problem

Drilldown count pill text (e.g. "2" in Quotes/Docs columns) becomes
unreadable when `dimBackground()` applies ANSI faint behind an overlay.
Same root cause as #833/#834: `dimBackground` converts bold (SGR 1) to
faint (SGR 2), leaving dark foreground text invisible on the surviving
bright background.

## Approach

Identical to the tab/house pill fix in PR #834: when an overlay is
active, switch from filled accent pill (`Drilldown()` — bright bg +
dark fg + bold) to accent-foreground-only (`AccentOutline()` — accent fg,
no bg). Faint then dims sky-blue text on the default terminal background,
which stays legible.

Thread an `overlayActive` bool through the render chain:

- `renderRows` → `renderRow` → `renderCell` → `renderPillCell`
- In `renderPillCell`: when `overlayActive && !deleted && !dimmed`,
  use `AccentOutline()` instead of `Drilldown()`
- Call sites:
  - `view.go` tableView — pass `m.hasActiveOverlay()`
  - `extraction_render.go` — pass `false` (inside an overlay, not behind)

## Files modified

- `internal/app/table.go` — add `overlayActive` param to render chain
- `internal/app/view.go` — pass `m.hasActiveOverlay()` at call site
- `internal/app/extraction_render.go` — pass `false` at call site
- `internal/app/dashboard_test.go` — test verifying pill switches from
  bg to fg under overlay
