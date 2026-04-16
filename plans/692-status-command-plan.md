<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# `micasa status` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps
> use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `micasa status` CLI command that prints overdue/upcoming
maintenance, open incidents, and active projects, exiting 0 (ok), 1
(error), or 2 (needs attention).

**Architecture:** Pure CLI command in `cmd/micasa/status.go` reusing
existing `data.Store` dashboard queries. Shared duration helpers moved
from `internal/app/` to `internal/data/` so CLI can use them without
importing the TUI package. Exit code 2 signaled via a typed
`exitError` sentinel.

**Tech Stack:** Go, cobra, `data.Store`, `text/tabwriter`,
`encoding/json`, testify

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/data/duration.go` | Create | `DateDiffDays`, `ShortDur`, `DaysText` (moved from app) |
| `internal/data/duration_test.go` | Create | Unit tests for duration helpers |
| `internal/app/table.go` | Modify | Remove `dateDiffDays`, call `data.DateDiffDays` |
| `internal/app/dashboard.go` | Modify | Remove `shortDur`, `daysText`, `daysUntil`, `pastDur`; call `data.*` |
| `cmd/micasa/status.go` | Create | `newStatusCmd`, `statusOpts`, `runStatus` |
| `cmd/micasa/status_test.go` | Create | All status command tests |
| `cmd/micasa/main.go` | Modify | Register status cmd, `exitError` type, handle in `main()` |

---

### Task 1: Move duration helpers to `internal/data/`

Extract `dateDiffDays`, `shortDur`, `daysText`, and `pastDur` from
the TUI package into `internal/data/duration.go` as exported
functions. This breaks the dependency that would otherwise force
`cmd/micasa/` to import `internal/app/`.

**Files:**
- Create: `internal/data/duration.go`
- Create: `internal/data/duration_test.go`
- Modify: `internal/app/table.go` (lines ~719-729)
- Modify: `internal/app/dashboard.go` (lines ~894-948)

- [ ] **Step 1: Write tests for duration helpers**

Create `internal/data/duration_test.go` with tests ported from the
existing `internal/app/dashboard_test.go` test cases, plus new edge
cases. Check existing tests first:

```go
// internal/data/duration_test.go
package data

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDateDiffDays(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 14, 15, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		target time.Time
		want   int
	}{
		{"same day", now, 0},
		{"tomorrow", now.AddDate(0, 0, 1), 1},
		{"yesterday", now.AddDate(0, 0, -1), -1},
		{"30 days ahead", now.AddDate(0, 0, 30), 30},
		{"30 days behind", now.AddDate(0, 0, -30), -30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, DateDiffDays(now, tt.target))
		})
	}
}

func TestDaysText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		days int
		want string
	}{
		{"zero", 0, "today"},
		{"positive 1d", 1, "1d"},
		{"negative 1d", -1, "1d"},
		{"positive 15d", 15, "15d"},
		{"negative 15d", -15, "15d"},
		{"months", 45, "1mo"},
		{"year", 400, "1y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, DaysText(tt.days))
		})
	}
}

func TestShortDur(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "now"},
		{"sub-minute", 30 * time.Second, "now"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 48 * time.Hour, "2d"},
		{"months", 60 * 24 * time.Hour, "2mo"},
		{"year", 400 * 24 * time.Hour, "1y"},
		{"negative", -5 * time.Minute, "5m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ShortDur(tt.d))
		})
	}
}

func TestPastDur(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "<1m", PastDur(10*time.Second))
	assert.Equal(t, "5m", PastDur(5*time.Minute))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestDateDiffDays|TestDaysText|TestShortDur|TestPastDur' ./internal/data/`
Expected: compilation error -- functions not defined yet.

- [ ] **Step 3: Create `internal/data/duration.go`**

```go
// internal/data/duration.go
package data

import (
	"fmt"
	"math"
	"time"
)

// DateDiffDays returns the number of calendar days from now to target,
// using each time's local Y/M/D. Positive means target is in the future.
func DateDiffDays(now, target time.Time) int {
	nowDate := time.Date(
		now.Year(), now.Month(), now.Day(),
		0, 0, 0, 0, time.UTC,
	)
	tgtDate := time.Date(
		target.Year(), target.Month(), target.Day(),
		0, 0, 0, 0, time.UTC,
	)
	return int(math.Round(tgtDate.Sub(nowDate).Hours() / 24))
}

// ShortDur returns a compressed duration string like "3d", "2mo", "1y".
func ShortDur(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

// DaysText returns a bare compressed duration like "5d" or "today".
func DaysText(days int) string {
	if days == 0 {
		return "today"
	}
	abs := days
	if abs < 0 {
		abs = -abs
	}
	return ShortDur(time.Duration(abs) * 24 * time.Hour)
}

// PastDur returns a compressed past-duration string. Sub-minute is "<1m".
func PastDur(d time.Duration) string {
	s := ShortDur(d)
	if s == "now" {
		return "<1m"
	}
	return s
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run 'TestDateDiffDays|TestDaysText|TestShortDur|TestPastDur' ./internal/data/`
Expected: PASS

- [ ] **Step 5: Update `internal/app/` to call `data.*`**

In `internal/app/table.go`, replace the `dateDiffDays` function body
with a call to `data.DateDiffDays`:

```go
func dateDiffDays(now, target time.Time) int {
	return data.DateDiffDays(now, target)
}
```

In `internal/app/dashboard.go`, replace bodies of `shortDur`,
`daysText`, `pastDur`, and `daysUntil` with delegations:

```go
func daysText(days int) string {
	return data.DaysText(days)
}

func shortDur(d time.Duration) string {
	return data.ShortDur(d)
}

func pastDur(d time.Duration) string {
	return data.PastDur(d)
}

func daysUntil(now, target time.Time) int {
	return data.DateDiffDays(now, target)
}
```

- [ ] **Step 6: Run full test suite**

Run: `go test -shuffle=on ./...`
Expected: all tests pass (existing behavior preserved).

- [ ] **Step 7: Commit**

Commit with type `refactor(data)`: move duration helpers to data
package for CLI reuse.

---

### Task 2: Implement `exitError` and `main()` plumbing

Add the `exitError` sentinel type and update `main()` to extract
custom exit codes from it.

**Files:**
- Modify: `cmd/micasa/main.go`

- [ ] **Step 1: Write test for exitError behavior**

In `cmd/micasa/status_test.go` (create file), write a test that
validates `exitError` semantics:

```go
// cmd/micasa/status_test.go
package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExitErrorCode(t *testing.T) {
	t.Parallel()
	err := exitError{code: 2}
	assert.Equal(t, 2, err.code)
	assert.Equal(t, "", err.Error())
}

func TestExitErrorUnwrap(t *testing.T) {
	t.Parallel()
	err := exitError{code: 2}
	var target exitError
	require.True(t, errors.As(err, &target))
	assert.Equal(t, 2, target.code)
}

func TestExtractExitCode_ExitError(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 2, extractExitCode(exitError{code: 2}))
}

func TestExtractExitCode_RegularError(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1, extractExitCode(errors.New("boom")))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestExitError|TestExtractExitCode' ./cmd/micasa/`
Expected: compilation error -- types not defined.

- [ ] **Step 3: Add `exitError` type and `extractExitCode` to `main.go`**

Add above the `main()` function:

```go
// exitError is a sentinel error that carries a process exit code.
// It is not printed to stderr by the error handler.
type exitError struct {
	code int
}

func (e exitError) Error() string { return "" }

// extractExitCode returns the exit code from an exitError, or 1 for
// any other error.
func extractExitCode(err error) int {
	var ee exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return 1
}
```

- [ ] **Step 4: Update `main()` to use `extractExitCode`**

Replace the error-handling block in `main()`:

```go
func main() {
	root := newRootCmd()
	if err := fang.Execute(
		context.Background(),
		root,
		fang.WithVersion(versionString()),
		fang.WithColorSchemeFunc(wongColorScheme),
		fang.WithNotifySignal(os.Interrupt),
		fang.WithErrorHandler(func(w io.Writer, _ fang.Styles, err error) {
			var ee exitError
			if errors.As(err, &ee) {
				return
			}
			fmt.Fprintln(w, err)
		}),
	); err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			os.Exit(130)
		}
		os.Exit(extractExitCode(err))
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -run 'TestExitError|TestExtractExitCode' ./cmd/micasa/`
Expected: PASS

- [ ] **Step 6: Run full test suite**

Run: `go test -shuffle=on ./...`
Expected: all tests pass.

- [ ] **Step 7: Commit**

Commit with type `refactor(cli)`: add exitError sentinel for custom
exit codes.

---

### Task 3: Implement `runStatus` core logic and register command

Build the status command: cobra registration, data loading, text and
JSON output, exit code logic.

**Files:**
- Create: `cmd/micasa/status.go`
- Modify: `cmd/micasa/main.go` (add to `newRootCmd`)

- [ ] **Step 1: Write tests for text output and exit codes**

Add to `cmd/micasa/status_test.go`:

Reuse `newTestStoreWithMigration` from `show_test.go` (same package).

```go
func TestStatusTextEmpty(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	err := runStatus(&buf, &statusOpts{days: 30}, store, now)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestStatusTextOverdue(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	pastDue := now.AddDate(0, 0, -10)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Replace filter",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)

	out := buf.String()
	assert.Contains(t, out, "=== OVERDUE ===")
	assert.Contains(t, out, "Replace filter")
	assert.Contains(t, out, "10d")
}

func TestStatusTextUpcoming(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	futureDue := now.AddDate(0, 0, 15)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Inspect roof",
		CategoryID: cats[0].ID,
		DueDate:    &futureDue,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "=== UPCOMING ===")
	assert.Contains(t, out, "Inspect roof")
	assert.Contains(t, out, "15d")
}

func TestStatusTextUpcomingDoesNotTriggerExit2(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	futureDue := now.AddDate(0, 0, 5)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Clean gutters",
		CategoryID: cats[0].ID,
		DueDate:    &futureDue,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	require.NoError(t, err)
}

func TestStatusTextIncidents(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.CreateIncident(&data.Incident{
		Title:    "Leaking faucet",
		Status:   data.IncidentStatusOpen,
		Severity: data.IncidentSeverityUrgent,
	}))

	var buf bytes.Buffer
	err := runStatus(&buf, &statusOpts{days: 30}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)

	out := buf.String()
	assert.Contains(t, out, "=== INCIDENTS ===")
	assert.Contains(t, out, "Leaking faucet")
	assert.Contains(t, out, "urgent")
}

func TestStatusTextActiveProjects(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Kitchen remodel",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusDelayed,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)

	out := buf.String()
	assert.Contains(t, out, "=== ACTIVE PROJECTS ===")
	assert.Contains(t, out, "Kitchen remodel")
	assert.Contains(t, out, "delayed")
}

func TestStatusUnderwayProjectDoesNotTriggerExit2(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Fence repair",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusInProgress,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "=== ACTIVE PROJECTS ===")
	assert.Contains(t, out, "Fence repair")
	assert.Contains(t, out, data.ProjectStatusInProgress)
}

func TestStatusDaysFlag(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	futureDue := now.AddDate(0, 0, 20)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Service heater",
		CategoryID: cats[0].ID,
		DueDate:    &futureDue,
	}))

	// With 10-day window: item at 20 days out should NOT appear
	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 10}, store, now)
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "Service heater")

	// With 30-day window: item at 20 days out SHOULD appear
	buf.Reset()
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Service heater")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestStatus' ./cmd/micasa/`
Expected: compilation error -- `runStatus`, `statusOpts` not defined.

- [ ] **Step 3: Implement `cmd/micasa/status.go`**

```go
// cmd/micasa/status.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/spf13/cobra"
)

type statusOpts struct {
	asJSON bool
	days   int
}

const (
	statusDaysMin = 1
	statusDaysMax = 365
)

func (o *statusOpts) validate() error {
	if o.days < statusDaysMin || o.days > statusDaysMax {
		return fmt.Errorf(
			"--days must be between %d and %d, got %d",
			statusDaysMin, statusDaysMax, o.days,
		)
	}
	return nil
}

func newStatusCmd() *cobra.Command {
	opts := &statusOpts{}

	cmd := &cobra.Command{
		Use:   "status [database-path]",
		Short: "Show overdue items, open incidents, and active projects",
		Long: `Print items that need attention and exit with code 2 if any
are found. Exit 0 means everything is on track. Useful for cron jobs,
shell prompts, and status bar widgets.`,
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.validate(); err != nil {
				return err
			}
			store, err := openExisting(dbPathFromEnvOrArg(args))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			return runStatus(cmd.OutOrStdout(), opts, store, time.Now())
		},
	}

	cmd.Flags().BoolVar(&opts.asJSON, "json", false,
		"Output JSON instead of human-readable text")
	cmd.Flags().IntVar(&opts.days, "days", 30,
		"Look-ahead window for upcoming items (1-365)")

	return cmd
}

func runStatus(
	w io.Writer,
	opts *statusOpts,
	store *data.Store,
	now time.Time,
) error {
	maintenance, err := store.ListMaintenanceWithSchedule()
	if err != nil {
		return fmt.Errorf("list maintenance: %w", err)
	}

	var overdue, upcoming []maintenanceStatus
	for _, m := range maintenance {
		nextDue := data.ComputeNextDue(m.LastServicedAt, m.IntervalMonths, m.DueDate)
		if nextDue == nil {
			continue
		}
		days := data.DateDiffDays(now, *nextDue)
		entry := maintenanceStatus{
			ID:        m.ID,
			Name:      m.Name,
			Category:  m.Category.Name,
			Appliance: m.Appliance.Name,
			NextDue:   *nextDue,
			Days:      days,
		}
		if days < 0 {
			entry.Days = -days
			overdue = append(overdue, entry)
		} else if days <= opts.days {
			upcoming = append(upcoming, entry)
		}
	}

	incidents, err := store.ListOpenIncidents()
	if err != nil {
		return fmt.Errorf("list incidents: %w", err)
	}

	projects, err := store.ListActiveProjects()
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	needsAttention := len(overdue) > 0 ||
		len(incidents) > 0 ||
		hasDelayedProject(projects)

	if opts.asJSON {
		if err := writeStatusJSON(w, overdue, upcoming, incidents, projects, needsAttention); err != nil {
			return err
		}
	} else {
		if err := writeStatusText(w, overdue, upcoming, incidents, projects, now); err != nil {
			return err
		}
	}

	if needsAttention {
		return exitError{code: 2}
	}
	return nil
}

type maintenanceStatus struct {
	ID        string
	Name      string
	Category  string
	Appliance string
	NextDue   time.Time
	Days      int
}

func hasDelayedProject(projects []data.Project) bool {
	for _, p := range projects {
		if p.Status == data.ProjectStatusDelayed {
			return true
		}
	}
	return false
}

// --- text output ---

func writeStatusText(
	w io.Writer,
	overdue, upcoming []maintenanceStatus,
	incidents []data.Incident,
	projects []data.Project,
	now time.Time,
) error {
	wrote := false
	if len(overdue) > 0 {
		if err := writeOverdueText(w, overdue); err != nil {
			return err
		}
		wrote = true
	}
	if len(upcoming) > 0 {
		if wrote {
			fmt.Fprintln(w)
		}
		if err := writeUpcomingText(w, upcoming); err != nil {
			return err
		}
		wrote = true
	}
	if len(incidents) > 0 {
		if wrote {
			fmt.Fprintln(w)
		}
		if err := writeIncidentsText(w, incidents, now); err != nil {
			return err
		}
		wrote = true
	}
	if len(projects) > 0 {
		if wrote {
			fmt.Fprintln(w)
		}
		if err := writeProjectsText(w, projects, now); err != nil {
			return err
		}
	}
	return nil
}

func writeOverdueText(w io.Writer, items []maintenanceStatus) error {
	if _, err := fmt.Fprintln(w, "=== OVERDUE ==="); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tOVERDUE")
	for _, m := range items {
		fmt.Fprintf(tw, "%s\t%s\n", m.Name, data.DaysText(m.Days))
	}
	return tw.Flush()
}

func writeUpcomingText(w io.Writer, items []maintenanceStatus) error {
	if _, err := fmt.Fprintln(w, "=== UPCOMING ==="); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tDUE")
	for _, m := range items {
		fmt.Fprintf(tw, "%s\t%s\n", m.Name, data.DaysText(m.Days))
	}
	return tw.Flush()
}

func writeIncidentsText(w io.Writer, incidents []data.Incident, now time.Time) error {
	if _, err := fmt.Fprintln(w, "=== INCIDENTS ==="); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TITLE\tSEVERITY\tREPORTED")
	for _, inc := range incidents {
		days := data.DateDiffDays(now, inc.DateNoticed)
		abs := days
		if abs < 0 {
			abs = -abs
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", inc.Title, inc.Severity, data.DaysText(abs))
	}
	return tw.Flush()
}

func writeProjectsText(w io.Writer, projects []data.Project, now time.Time) error {
	if _, err := fmt.Fprintln(w, "=== ACTIVE PROJECTS ==="); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TITLE\tSTATUS\tSTARTED")
	for _, p := range projects {
		started := "-"
		if p.StartDate != nil {
			days := data.DateDiffDays(now, *p.StartDate)
			abs := days
			if abs < 0 {
				abs = -abs
			}
			started = data.DaysText(abs)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", p.Title, p.Status, started)
	}
	return tw.Flush()
}

// --- JSON output ---

func writeStatusJSON(
	w io.Writer,
	overdue, upcoming []maintenanceStatus,
	incidents []data.Incident,
	projects []data.Project,
	needsAttention bool,
) error {
	result := statusJSON{
		Overdue:        make([]maintenanceJSON, 0, len(overdue)),
		Upcoming:       make([]maintenanceJSON, 0, len(upcoming)),
		Incidents:      make([]incidentJSON, 0, len(incidents)),
		ActiveProjects: make([]projectJSON, 0, len(projects)),
		NeedsAttention: needsAttention,
	}

	for _, m := range overdue {
		result.Overdue = append(result.Overdue, maintenanceJSON{
			ID:          m.ID,
			Name:        m.Name,
			Category:    m.Category,
			Appliance:   m.Appliance,
			NextDue:     m.NextDue.Format("2006-01-02"),
			DaysOverdue: m.Days,
		})
	}
	for _, m := range upcoming {
		result.Upcoming = append(result.Upcoming, maintenanceJSON{
			ID:           m.ID,
			Name:         m.Name,
			Category:     m.Category,
			Appliance:    m.Appliance,
			NextDue:      m.NextDue.Format("2006-01-02"),
			DaysUntilDue: m.Days,
		})
	}
	for _, inc := range incidents {
		result.Incidents = append(result.Incidents, incidentJSON{
			ID:          inc.ID,
			Title:       inc.Title,
			Status:      inc.Status,
			Severity:    inc.Severity,
			DateNoticed: inc.DateNoticed.Format("2006-01-02"),
		})
	}
	for _, p := range projects {
		pj := projectJSON{
			ID:     p.ID,
			Title:  p.Title,
			Status: p.Status,
		}
		if p.StartDate != nil {
			s := p.StartDate.Format("2006-01-02")
			pj.StartDate = s
		}
		result.ActiveProjects = append(result.ActiveProjects, pj)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encode status JSON: %w", err)
	}
	return nil
}

type statusJSON struct {
	Overdue        []maintenanceJSON `json:"overdue"`
	Upcoming       []maintenanceJSON `json:"upcoming"`
	Incidents      []incidentJSON    `json:"incidents"`
	ActiveProjects []projectJSON     `json:"active_projects"`
	NeedsAttention bool              `json:"needs_attention"`
}

type maintenanceJSON struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Category     string `json:"category"`
	Appliance    string `json:"appliance"`
	NextDue      string `json:"next_due"`
	DaysOverdue  int    `json:"days_overdue,omitempty"`
	DaysUntilDue int    `json:"days_until_due,omitempty"`
}

type incidentJSON struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	DateNoticed string `json:"date_noticed"`
}

type projectJSON struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	StartDate string `json:"start_date,omitempty"`
}
```

- [ ] **Step 4: Register command in `newRootCmd`**

In `cmd/micasa/main.go`, add `newStatusCmd()` to the `AddCommand`
call inside `newRootCmd()`:

```go
root.AddCommand(
	newDemoCmd(),
	newBackupCmd(),
	newConfigCmd(),
	newProCmd(),
	newMCPCmd(),
	newShowCmd(),
	newQueryCmd(),
	newGenCLIRefCmd(),
	newStatusCmd(),
)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -run 'TestStatus' ./cmd/micasa/`
Expected: PASS

- [ ] **Step 6: Run full test suite**

Run: `go test -shuffle=on ./...`
Expected: all tests pass.

- [ ] **Step 7: Commit**

Commit with type `feat(cli)`: add `micasa status` command for
headless health checks. Closes #692.

---

### Task 4: Add JSON output tests and error-path coverage

**Files:**
- Modify: `cmd/micasa/status_test.go`

- [ ] **Step 1: Write JSON and error-path tests**

Add to `cmd/micasa/status_test.go`:

```go
func TestStatusJSONEmpty(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	err := runStatus(&buf, &statusOpts{asJSON: true, days: 30}, store, now)
	require.NoError(t, err)

	var result statusJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Empty(t, result.Overdue)
	assert.Empty(t, result.Upcoming)
	assert.Empty(t, result.Incidents)
	assert.Empty(t, result.ActiveProjects)
	assert.False(t, result.NeedsAttention)
}

func TestStatusJSONOverdue(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	pastDue := now.AddDate(0, 0, -5)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Change filter",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{asJSON: true, days: 30}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)

	var result statusJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result.Overdue, 1)
	assert.Equal(t, "Change filter", result.Overdue[0].Name)
	assert.Equal(t, 5, result.Overdue[0].DaysOverdue)
	assert.True(t, result.NeedsAttention)
}

func TestStatusJSONNeedsAttentionFalseOnCleanDB(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	err := runStatus(&buf, &statusOpts{asJSON: true, days: 30}, store, now)
	require.NoError(t, err)

	var result statusJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.False(t, result.NeedsAttention)
}

func TestStatusValidateDays(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		days int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too large", 366},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := &statusOpts{days: tt.days}
			err := opts.validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--days")
		})
	}
}

func TestStatusValidateDaysBoundaries(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&statusOpts{days: 1}).validate())
	assert.NoError(t, (&statusOpts{days: 365}).validate())
}

func TestStatusCLIMissingDB(t *testing.T) {
	t.Parallel()
	_, err := executeCLI("status", "/nonexistent/path.db")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test -run 'TestStatusJSON|TestStatusValidate|TestStatusCLIMissing' ./cmd/micasa/`
Expected: PASS

- [ ] **Step 3: Run full test suite**

Run: `go test -shuffle=on ./...`
Expected: all tests pass.

- [ ] **Step 4: Commit**

Commit with type `test(cli)`: add JSON and error-path coverage for
status command.

---

### Task 5: Integration test via `executeCLI`

Verify the command works end-to-end through cobra, including exit
code behavior via the error returned by `executeCLI`.

**Files:**
- Modify: `cmd/micasa/status_test.go`

- [ ] **Step 1: Write integration tests**

Add to `cmd/micasa/status_test.go`:

```go
func TestStatusCLITextClean(t *testing.T) {
	t.Parallel()
	src := createTestDB(t)
	out, err := executeCLI("status", src)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestStatusCLITextOverdue(t *testing.T) {
	t.Parallel()
	src := createTestDB(t)
	store, err := data.Open(src)
	require.NoError(t, err)
	cats, catErr := store.MaintenanceCategories()
	require.NoError(t, catErr)

	pastDue := time.Now().AddDate(0, 0, -7)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "CLI overdue item",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))
	require.NoError(t, store.Close())

	out, err := executeCLI("status", src)
	require.Error(t, err)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)
	assert.Contains(t, out, "OVERDUE")
	assert.Contains(t, out, "CLI overdue item")
}

func TestStatusCLIJSONClean(t *testing.T) {
	t.Parallel()
	src := createTestDB(t)
	out, err := executeCLI("status", "--json", src)
	require.NoError(t, err)

	var result statusJSON
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.False(t, result.NeedsAttention)
}

func TestStatusCLIDaysValidation(t *testing.T) {
	t.Parallel()
	src := createTestDB(t)
	_, err := executeCLI("status", "--days", "0", src)
	require.Error(t, err)
	assert.ErrorContains(t, err, "--days")

	_, err = executeCLI("status", "--days", "-1", src)
	require.Error(t, err)

	_, err = executeCLI("status", "--days", "366", src)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test -run 'TestStatusCLI' ./cmd/micasa/`
Expected: PASS

- [ ] **Step 3: Run full test suite**

Run: `go test -shuffle=on ./...`
Expected: all tests pass.

- [ ] **Step 4: Commit**

Commit with type `test(cli)`: add integration tests for status
command.

---

### Task 6: Lint, coverage verification, final check

Run linters and verify coverage before the feature is complete.

- [ ] **Step 1: Run linter**

Run: `golangci-lint run ./...`
Expected: no warnings.

- [ ] **Step 2: Run coverage check**

Run: `go test -coverprofile cover.out ./cmd/micasa/ ./internal/data/`
then `go tool cover -func cover.out`
Verify new functions in `status.go` and `duration.go` are exercised.

- [ ] **Step 3: Fix any gaps**

Add tests for any uncovered paths found in step 2.

- [ ] **Step 4: Final full test run**

Run: `go test -shuffle=on ./...`
Expected: all tests pass.

- [ ] **Step 5: Commit any remaining fixes**

Commit with appropriate type.
