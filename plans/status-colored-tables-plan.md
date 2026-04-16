<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Status Colored Tables Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace plain tabwriter output in `micasa status` with styled lipgloss/table tables using Wong colorblind-safe palette.

**Architecture:** Add `cliStyles` struct to `theme.go` with Wong palette styles. Add `isDark` field to `statusOpts`. Replace each `write*Text` function with lipgloss/table rendering. Tests use `ansi.Strip()` for content assertions.

**Tech Stack:** `charm.land/lipgloss/v2/table`, `github.com/charmbracelet/x/ansi` (both already dependencies)

---

### Task 1: Add `cliStyles` to `theme.go`

**Files:**
- Modify: `cmd/micasa/theme.go`

- [ ] **Step 1: Add `cliStyles` struct and constructor**

```go
type cliStyles struct {
	sectionHeader lipgloss.Style
	tableHeader   lipgloss.Style
	danger        lipgloss.Style
	warning       lipgloss.Style
	success       lipgloss.Style
	muted         lipgloss.Style
	border        lipgloss.Style
}

func newCLIStyles(isDark bool) cliStyles {
	c := lipgloss.LightDark(isDark)
	blue := c(lipgloss.Color("#0072B2"), lipgloss.Color("#56B4E9"))
	orange := c(lipgloss.Color("#D55E00"), lipgloss.Color("#E69F00"))
	green := c(lipgloss.Color("#007A5A"), lipgloss.Color("#009E73"))
	vermillion := c(lipgloss.Color("#CC3311"), lipgloss.Color("#D55E00"))
	rose := c(lipgloss.Color("#AA4499"), lipgloss.Color("#CC79A7"))
	dim := c(lipgloss.Color("#4B5563"), lipgloss.Color("#6B7280"))
	border := c(lipgloss.Color("#D1D5DB"), lipgloss.Color("#374151"))

	return cliStyles{
		sectionHeader: lipgloss.NewStyle().Bold(true).Foreground(blue),
		tableHeader:   lipgloss.NewStyle().Bold(true).Foreground(dim),
		danger:        lipgloss.NewStyle().Foreground(vermillion),
		warning:       lipgloss.NewStyle().Foreground(orange),
		success:       lipgloss.NewStyle().Foreground(green),
		muted:         lipgloss.NewStyle().Foreground(rose),
		border:        lipgloss.NewStyle().Foreground(border),
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/micasa/`

- [ ] **Step 3: Commit**

Message: `refactor(cli): add cliStyles struct for status output theming`

---

### Task 2: Add `isDark` to `statusOpts` and plumb through

**Files:**
- Modify: `cmd/micasa/status.go`

- [ ] **Step 1: Add `isDark` field to `statusOpts`**

Add `isDark bool` field to the `statusOpts` struct.

- [ ] **Step 2: Set `isDark` in cobra handler**

In `newStatusCmd`, set `opts.isDark` before calling `runStatus`:

```go
RunE: func(cmd *cobra.Command, args []string) error {
	if err := opts.validate(); err != nil {
		return err
	}
	opts.isDark = lipgloss.HasDarkBackground(os.Stdin, os.Stderr)
	store, err := openExisting(dbPathFromEnvOrArg(args))
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return runStatus(cmd.OutOrStdout(), opts, store, time.Now())
},
```

Add `"os"` and `lipgloss` imports as needed.

- [ ] **Step 3: Pass styles through the entire text path**

Update `writeStatusText` and all four section helpers to accept
`cliStyles` as a parameter. The section helpers accept it but ignore
it for now — they still use tabwriter internally. This keeps the code
compiling and tests passing while we convert one section at a time.

In `runStatus`:
```go
if err := writeStatusText(w, newCLIStyles(opts.isDark), overdue, upcoming, incidents, projects, now); err != nil {
	return err
}
```

Update signatures (implementations unchanged for now):
```go
func writeStatusText(w io.Writer, styles cliStyles, ...) error {
	// pass styles to each section helper
}
func writeOverdueText(w io.Writer, styles cliStyles, items []maintenanceStatus) error { ... }
func writeUpcomingText(w io.Writer, styles cliStyles, items []maintenanceStatus) error { ... }
func writeIncidentsText(w io.Writer, styles cliStyles, incidents []data.Incident, now time.Time) error { ... }
func writeProjectsText(w io.Writer, styles cliStyles, projects []data.Project, now time.Time) error { ... }
```

- [ ] **Step 4: Verify it compiles**

Run: `go build ./cmd/micasa/`

- [ ] **Step 5: Run existing tests to confirm no breakage**

Run: `go test -shuffle=on ./cmd/micasa/ -run TestStatus`

Tests should pass — section helpers accept `styles` but still use
tabwriter internally, so output is unchanged.

- [ ] **Step 6: Commit**

Message: `refactor(cli): plumb isDark and cliStyles through status text path`

---

### Task 3: Replace `writeOverdueText` with lipgloss/table

**Files:**
- Modify: `cmd/micasa/status.go`
- Modify: `cmd/micasa/status_test.go`

- [ ] **Step 1: Update test to use `ansi.Strip()` — overdue tests**

Add `"github.com/charmbracelet/x/ansi"` import to `status_test.go`.

In `TestStatusTextOverdue`, change:
```go
out := buf.String()
```
to:
```go
out := ansi.Strip(buf.String())
```

Update the assertion — replace `"=== OVERDUE ==="` with `"OVERDUE"` (new
styled headers won't have `===`).

Do the same for `TestStatusOverdueSortOrder`.

- [ ] **Step 2: Run updated tests — should still pass with old impl**

Run: `go test -shuffle=on ./cmd/micasa/ -run "TestStatusTextOverdue|TestStatusOverdueSortOrder"`

The `ansi.Strip()` on non-ANSI text is a no-op, so existing impl still passes.

- [ ] **Step 3: Replace `writeOverdueText` implementation**

```go
func writeOverdueText(w io.Writer, styles cliStyles, items []maintenanceStatus) error {
	if _, err := fmt.Fprintln(w, styles.sectionHeader.Render("OVERDUE")); err != nil {
		return fmt.Errorf("write overdue header: %w", err)
	}
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(styles.border).
		Headers("NAME", "OVERDUE").
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return styles.tableHeader
			}
			if col == 1 {
				return styles.danger
			}
			return lipgloss.NewStyle()
		})
	for _, m := range items {
		t.Row(m.Name, data.DaysText(m.Days))
	}
	if _, err := fmt.Fprintln(w, t); err != nil {
		return fmt.Errorf("write overdue table: %w", err)
	}
	return nil
}
```

Add `"charm.land/lipgloss/v2/table"` import to `status.go`.
Keep `"text/tabwriter"` until all sections are converted.

- [ ] **Step 4: Run overdue tests**

Run: `go test -shuffle=on ./cmd/micasa/ -run "TestStatusTextOverdue|TestStatusOverdueSortOrder"`

- [ ] **Step 5: Commit**

Message: `feat(cli): use lipgloss/table for overdue section in status output`

---

### Task 4: Replace `writeUpcomingText` with lipgloss/table

**Files:**
- Modify: `cmd/micasa/status.go`
- Modify: `cmd/micasa/status_test.go`

- [ ] **Step 1: Update upcoming test assertions**

In `TestStatusTextUpcoming`: wrap `buf.String()` with `ansi.Strip()`,
replace `"=== UPCOMING ==="` with `"UPCOMING"`.

`TestStatusUpcomingDoesNotTriggerExit2` has no text assertions — no
changes needed there.

- [ ] **Step 2: Replace `writeUpcomingText` implementation**

```go
func writeUpcomingText(w io.Writer, styles cliStyles, items []maintenanceStatus) error {
	if _, err := fmt.Fprintln(w, styles.sectionHeader.Render("UPCOMING")); err != nil {
		return fmt.Errorf("write upcoming header: %w", err)
	}
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(styles.border).
		Headers("NAME", "DUE").
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return styles.tableHeader
			}
			return lipgloss.NewStyle()
		})
	for _, m := range items {
		t.Row(m.Name, data.DaysText(m.Days))
	}
	if _, err := fmt.Fprintln(w, t); err != nil {
		return fmt.Errorf("write upcoming table: %w", err)
	}
	return nil
}
```


- [ ] **Step 3: Run upcoming tests**

Run: `go test -shuffle=on ./cmd/micasa/ -run "TestStatusTextUpcoming|TestStatusUpcomingDoesNotTriggerExit2"`

- [ ] **Step 4: Commit**

Message: `feat(cli): use lipgloss/table for upcoming section in status output`

---

### Task 5: Replace `writeIncidentsText` with lipgloss/table

**Files:**
- Modify: `cmd/micasa/status.go`
- Modify: `cmd/micasa/status_test.go`

- [ ] **Step 1: Update incidents test assertions**

In `TestStatusTextIncidents`: wrap `buf.String()` with `ansi.Strip()`,
replace `"=== INCIDENTS ==="` with `"INCIDENTS"`.

- [ ] **Step 2: Replace `writeIncidentsText` implementation**

```go
func writeIncidentsText(w io.Writer, styles cliStyles, incidents []data.Incident, now time.Time) error {
	if _, err := fmt.Fprintln(w, styles.sectionHeader.Render("INCIDENTS")); err != nil {
		return fmt.Errorf("write incidents header: %w", err)
	}
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(styles.border).
		Headers("TITLE", "SEVERITY", "REPORTED").
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return styles.tableHeader
			}
			if col == 1 && row >= 0 && row < len(incidents) {
				switch incidents[row].Severity {
				case data.IncidentSeverityUrgent:
					return styles.danger
				case data.IncidentSeveritySoon:
					return styles.warning
				case data.IncidentSeverityWhenever:
					return styles.muted
				}
			}
			return lipgloss.NewStyle()
		})
	for _, inc := range incidents {
		days := data.DateDiffDays(now, inc.DateNoticed)
		if days < 0 {
			days = -days
		}
		t.Row(inc.Title, inc.Severity, data.DaysText(days))
	}
	if _, err := fmt.Fprintln(w, t); err != nil {
		return fmt.Errorf("write incidents table: %w", err)
	}
	return nil
}
```


- [ ] **Step 3: Run incidents tests**

Run: `go test -shuffle=on ./cmd/micasa/ -run TestStatusTextIncidents`

- [ ] **Step 4: Commit**

Message: `feat(cli): use lipgloss/table for incidents section in status output`

---

### Task 6: Replace `writeProjectsText` with lipgloss/table

**Files:**
- Modify: `cmd/micasa/status.go`
- Modify: `cmd/micasa/status_test.go`

- [ ] **Step 1: Update projects test assertions**

In `TestStatusTextActiveProjects`, `TestStatusUnderwayProjectDoesNotTriggerExit2`,
and `TestStatusTextProjectWithStartDate`: wrap `buf.String()` with
`ansi.Strip()`, replace `"=== ACTIVE PROJECTS ==="` with `"ACTIVE PROJECTS"`.

- [ ] **Step 2: Replace `writeProjectsText` implementation**

```go
func writeProjectsText(w io.Writer, styles cliStyles, projects []data.Project, now time.Time) error {
	if _, err := fmt.Fprintln(w, styles.sectionHeader.Render("ACTIVE PROJECTS")); err != nil {
		return fmt.Errorf("write projects header: %w", err)
	}
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(styles.border).
		Headers("TITLE", "STATUS", "STARTED").
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return styles.tableHeader
			}
			if col == 1 && row >= 0 && row < len(projects) {
				switch projects[row].Status {
				case data.ProjectStatusDelayed:
					return styles.danger
				case data.ProjectStatusInProgress:
					return styles.success
				}
			}
			return lipgloss.NewStyle()
		})
	for _, p := range projects {
		started := "-"
		if p.StartDate != nil {
			days := data.DateDiffDays(now, *p.StartDate)
			if days < 0 {
				days = -days
			}
			started = data.DaysText(days)
		}
		t.Row(p.Title, p.Status, started)
	}
	if _, err := fmt.Fprintln(w, t); err != nil {
		return fmt.Errorf("write projects table: %w", err)
	}
	return nil
}
```


- [ ] **Step 3: Remove `"text/tabwriter"` import**

All four section functions now use lipgloss/table. Remove the unused
`text/tabwriter` import from `status.go`.

- [ ] **Step 4: Run all projects tests and full status suite**

Run: `go test -shuffle=on ./cmd/micasa/ -run TestStatus`

- [ ] **Step 5: Commit**

Message: `feat(cli): use lipgloss/table for projects section in status output`

---

### Task 7: Update multi-section and CLI integration tests

**Files:**
- Modify: `cmd/micasa/status_test.go`

- [ ] **Step 1: Update `TestStatusTextMultipleSections`**

Wrap `buf.String()` with `ansi.Strip()`. Replace all `=== X ===`
assertions with bare header names: `"OVERDUE"`, `"UPCOMING"`,
`"INCIDENTS"`, `"ACTIVE PROJECTS"`.

- [ ] **Step 2: Update `TestStatusTextEmpty`**

This test expects empty output on clean DB. With lipgloss/table, empty
output should still be empty (no sections rendered). Wrap with
`ansi.Strip()` for safety.

- [ ] **Step 3: Update `TestStatusDaysFlag`**

Wrap both `buf.String()` calls with `ansi.Strip()` for consistency.
Assertions use `Contains`/`NotContains` on item names, which would
match through ANSI escapes anyway, but stripping keeps the test pattern
uniform.

- [ ] **Step 4: Update CLI integration tests**

In `TestStatusCLITextClean` and `TestStatusCLITextOverdue`: wrap output
with `ansi.Strip()`. The CLI tests go through `executeCLI` which uses
`cobra.SetOut(&stdout)` — `isDark` will be detected but output gets
stripped anyway.

- [ ] **Step 5: Update `TestStatusTextWriteError`**

This test uses `failWriter{}`. The new implementation calls
`fmt.Fprintln(w, t)` which will still fail on a broken writer. No
content changes needed, but verify it still passes.

- [ ] **Step 6: Run full test suite**

Run: `go test -shuffle=on ./cmd/micasa/ -run TestStatus`

- [ ] **Step 7: Commit**

Message: `test(cli): update status tests for lipgloss/table output`

---

### Task 8: Add ANSI smoke test

**Files:**
- Modify: `cmd/micasa/status_test.go`

- [ ] **Step 1: Write smoke test**

```go
func TestStatusTextContainsANSI(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	pastDue := now.AddDate(0, 0, -5)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "ANSI test item",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30, isDark: true}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)

	assert.Contains(t, buf.String(), "\x1b[",
		"output should contain ANSI escape sequences")
}
```

- [ ] **Step 2: Run smoke test**

Run: `go test -shuffle=on ./cmd/micasa/ -run TestStatusTextContainsANSI`

- [ ] **Step 3: Run full test suite**

Run: `go test -shuffle=on ./cmd/micasa/`

- [ ] **Step 4: Commit**

Message: `test(cli): add ANSI smoke test for styled status output`

---

### Task 9: Manual verification

- [ ] **Step 1: Build and run with demo data**

Run: `go run ./cmd/micasa demo --seed-only /tmp/status-demo.db`
Run: `go run ./cmd/micasa status /tmp/status-demo.db`

Visually confirm: colored section headers, rounded table borders,
vermillion on overdue days, severity colors on incidents, status
colors on projects.

- [ ] **Step 2: Verify JSON output unchanged**

Run: `go run ./cmd/micasa status --json /tmp/status-demo.db`

JSON output should be identical to before — no style changes.

- [ ] **Step 3: Clean up**

Run: `rm /tmp/status-demo.db`

Note: `NO_COLOR` support is not implemented — the plan uses `fmt.Fprintln`
which always emits ANSI escapes when styles are set. If `NO_COLOR`
support becomes a requirement, wrap the writer with
`colorprofile.NewWriter(w, os.Environ())` before writes, and update the
ANSI smoke test to force `colorprofile.TrueColor` on its writer.
