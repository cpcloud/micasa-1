<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Dynamic Overlay & Form Sizing

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the dashboard overlay and house form scale proportionally to terminal width instead of using hard-capped maximums.

**Architecture:** Replace the fixed `72`-column cap on `overlayContentWidth()` and the fixed `60`-column cap on the house form with proportional calculations that grow on wide terminals while preserving existing behavior at standard (120) and narrow (80-85) widths. Both functions already have `m.width` / `m.effectiveWidth()` plumbing — only the clamping math changes.

**Tech Stack:** Go, lipgloss, Bubbletea, huh, testify

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/app/model.go:1284-1296` | Modify | `overlayContentWidth()` — proportional cap replaces fixed 72 |
| `internal/app/forms.go:268-272` | Modify | House form width — extract to `houseFormWidth()`, scale up |
| `internal/app/dashboard_test.go:1358-1378` | Modify | `TestOverlayContentWidth` — add wide-terminal cases |
| `internal/app/form_save_test.go` | Modify | Add `TestHouseFormWidth` table test |

---

### Task 1: Update `overlayContentWidth()` to scale proportionally

**Files:**
- Modify: `internal/app/model.go:1284-1296`
- Modify: `internal/app/dashboard_test.go:1358-1378`

The formula: `w = ew - 12`, capped by `max(ew*3/5, 72)`, floored at 30. The `max(..., 72)` ensures overlays never shrink below the old cap at standard and narrow widths — the proportional scaling only kicks in above ~120 columns.

Math verification:
- 200 → w=188, cap=max(120,72)=120, result=120 (was 72)
- 160 → w=148, cap=max(96,72)=96, result=96 (was 72)
- 120 → w=108, cap=max(72,72)=72, result=72 (unchanged)
- 100 → w=88, cap=max(60,72)=72, result=72 (unchanged)
- 85 → w=73, cap=max(51,72)=72, result=72 (unchanged)
- 80 → w=68, cap=max(48,72)=72, result=68 (unchanged)
- 60 → w=48, cap=max(36,72)=72, result=48 (unchanged)
- 30 → w=18, cap=max(18,72)=72, result=18→clamped to 30 (unchanged)

- [ ] **Step 1: Update the test expectations**

In `internal/app/dashboard_test.go`, replace the `TestOverlayContentWidth` function body:

```go
func TestOverlayContentWidth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		termWidth int
		want      int
	}{
		{"ultra-wide scales to 3/5", 200, 120},
		{"wide terminal scales", 160, 96},
		{"standard terminal unchanged", 120, 72},
		{"normal terminal unchanged", 100, 72},
		{"85-col terminal unchanged", 85, 72},
		{"80-col terminal", 80, 68},
		{"narrow terminal unchanged", 60, 48},
		{"very narrow caps at 30", 30, 30},
		{"minimum clamp", 20, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.width = tt.termWidth
			assert.Equal(t, tt.want, m.overlayContentWidth())
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestOverlayContentWidth -shuffle=on ./internal/app/`
Expected: FAIL — the 200-wide case returns 72 (old hard cap) instead of 120.

- [ ] **Step 3: Implement proportional scaling in `overlayContentWidth()`**

In `internal/app/model.go`, replace lines 1284-1296 (the comment + function):

```go
// overlayContentWidth returns the clamped content width for overlay boxes
// (dashboard, note preview, ops tree). Accounts for border (2), padding (4),
// and breathing room (6) = 12 total. On wide terminals the cap scales to
// 3/5 of terminal width so overlays grow proportionally; on standard and
// narrow terminals the old 72-column cap is preserved as a floor.
func (m *Model) overlayContentWidth() int {
	ew := m.effectiveWidth()
	w := ew - 12
	// Proportional cap that never drops below the legacy 72-column max.
	w = min(w, max(ew*3/5, 72))
	return max(w, 30)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run TestOverlayContentWidth -shuffle=on ./internal/app/`
Expected: PASS

- [ ] **Step 5: Run the full test suite to check for regressions**

Run: `go test -shuffle=on ./internal/app/`
Expected: PASS — the note preview overlay and ops tree overlay both call `overlayContentWidth()` and should still work since they pass the result to `OverlayBox().Width()`. The ops tree also does its own expansion from this starting width if hints or preview tables need more room.

- [ ] **Step 6: Commit**

```
refactor(ui): scale overlay width proportionally on wide terminals

overlayContentWidth() was hard-capped at 72 columns, making the
dashboard and note preview overlays look small on wide terminals
(150+). Replace the fixed cap with max(ew*3/5, 72) so overlays
grow proportionally while preserving existing behavior at standard
(120) and narrow widths.
```

---

### Task 2: Scale house form width proportionally

**Files:**
- Modify: `internal/app/forms.go:268-272`
- Modify: `internal/app/form_save_test.go`

Extract the width calculation into a `houseFormWidth()` method so it can be tested directly (huh.Form does not expose its width after `WithWidth()`).

The formula: `max(60, min(ew/2, 80))`, then scale down if terminal is too narrow (same as old code). This keeps the form at 60 for ≤120-col terminals (unchanged), grows to 80 on ultra-wide, and preserves the narrow-terminal fallback.

Note: the old code used `m.width` directly with a `m.width > 0` guard. The new code uses `m.effectiveWidth()` which returns `defaultWidth` (80) when `m.width == 0`, making the old `> 0` guard tautological — so we drop it. The narrow-terminal branch (`ew < formWidth+10`) is unreachable when `m.width == 0` because `effectiveWidth()` returns 80 and `formWidth` is 60, so `80 < 70` is false. A `width=0` test case locks this in.

Math verification (tracing through `houseFormWidth()`):
- 200 → min(100,80)=80, max(80,60)=80, 200<90? no → **80** (was 60)
- 160 → min(80,80)=80, max(80,60)=80, 160<90? no → **80** (was 60)
- 120 → min(60,80)=60, max(60,60)=60, 120<70? no → **60** (unchanged)
- 100 → min(50,80)=50, max(50,60)=60, 100<70? no → **60** (unchanged)
- 80 → min(40,80)=40, max(40,60)=60, 80<70? no → **60** (unchanged)
- 65 → min(32,80)=32, max(32,60)=60, 65<70? yes → 65-10=55 → **55** (unchanged)
- 50 → min(25,80)=25, max(25,60)=60, 50<70? yes → 50-10=40 → **40** (unchanged)
- 35 → min(17,80)=17, max(17,60)=60, 35<70? yes → max(35-10,30)=max(25,30)=**30** (was 25)
- 0 → effectiveWidth()=80, same as 80 case → **60**

- [ ] **Step 1: Write a test for house form width at various terminal sizes**

Add to `internal/app/form_save_test.go`:

```go
func TestHouseFormWidth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		termWidth int
		want      int
	}{
		{"ultra-wide caps at 80", 200, 80},
		{"wide terminal", 160, 80},
		{"standard terminal unchanged", 120, 60},
		{"normal terminal unchanged", 100, 60},
		{"minimum size unchanged", 80, 60},
		{"narrow scales down", 65, 55},
		{"very narrow scales down", 50, 40},
		{"extremely narrow floors at 30", 35, 30},
		{"zero-width uses default", 0, 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.width = tt.termWidth
			assert.Equal(t, tt.want, m.houseFormWidth())
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestHouseFormWidth -shuffle=on ./internal/app/`
Expected: FAIL — `houseFormWidth()` method does not exist yet.

- [ ] **Step 3: Extract `houseFormWidth()` and implement proportional scaling**

In `internal/app/forms.go`, add the new method and update `startHouseForm()`.

Add the method (above `startHouseForm()`):

```go
// houseFormWidth returns the form width for the house profile form.
// Scales to half the terminal width on wide terminals, clamped to [30, 80].
// At standard widths (≤120) this returns 60, preserving the old default.
func (m *Model) houseFormWidth() int {
	ew := m.effectiveWidth()
	formWidth := max(min(ew/2, 80), 60)
	// Scale down when the terminal can't fit the form plus breathing room.
	// Reached when m.width is a small positive value (e.g. in tests or on
	// tiny terminals). effectiveWidth() returns defaultWidth (80) only when
	// m.width is zero; positive values below 70 enter this path.
	if ew < formWidth+10 {
		formWidth = max(ew-10, 30)
	}
	return formWidth
}
```

Replace the old width logic in `startHouseForm()` (lines 268-272):

```go
	form.WithWidth(m.houseFormWidth())
```

This replaces the five lines at 268-272:
```go
	formWidth := 60
	if m.width > 0 && m.width < formWidth+10 {
		formWidth = m.width - 10
	}
	form.WithWidth(formWidth)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run TestHouseFormWidth -shuffle=on ./internal/app/`
Expected: PASS

- [ ] **Step 5: Run the full test suite**

Run: `go test -shuffle=on ./internal/app/`
Expected: PASS — existing house form tests (`TestUserEditsHouseProfileAndSavesWithCtrlS`, etc.) should be unaffected since the default test model width is 120, which gives the same 60-col result.

- [ ] **Step 6: Commit**

```
refactor(ui): scale house form width on wide terminals

The house profile form was hard-capped at 60 columns regardless of
terminal size. On wide terminals (150+) this left the form looking
small and lost in the center of the screen. Extract the width
calculation into houseFormWidth() and scale to half the terminal
width, clamped to [30, 80], preserving the old 60-column default
at standard widths (≤120).
```

---

### Task 3: Visual verification at multiple terminal sizes

The automated tests in Tasks 1 and 2 are the authoritative verification. This task is a sanity check to confirm the visual result looks right.

- [ ] **Step 1: Verify at 200x50 (ultra-wide)**

Launch `go run ./cmd/micasa demo` in tmux at 200x50. Open the dashboard (Shift+D). Confirm the overlay is visibly wider than 72 columns (~120 cols). Close dashboard. Enter edit mode (i), press p for house profile. Confirm the form is ~80 columns wide and centered.

- [ ] **Step 2: Verify at 120x40 (standard)**

Resize tmux to 120x40. Repeat. Dashboard should be 72 columns. House form should be 60 columns. Both should look identical to the old behavior at this size.

- [ ] **Step 3: Verify at 85x25 (near minimum)**

Resize to 85x25. Dashboard should be 72 columns (cap floor). House form should be 60 columns. Everything should still fit without wrapping.

- [ ] **Step 4: Verify at 80x24 (minimum)**

Resize to 80x24. Dashboard should be 68 columns (80-12). House form should be 60 columns. The "terminal too small" screen should NOT appear.
