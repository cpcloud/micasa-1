<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Test Suite Interaction Audit

Issue: #532

## Audit Scope

All 76 `_test.go` files across the repo. The user-interaction requirement
(tests must exercise the same path a real user triggers) applies to
`internal/app/` -- the TUI layer. Non-app packages (`data`, `config`,
`extract`, `llm`, `fake`, `cmd`) are library/infrastructure code with no
keypress interaction path; they are correctly tested at their API level and
excluded from this audit.

## Methodology

- "User interaction" = tests that use `sendKey`, `openAddForm`,
  `loadDashboardAt`, or equivalent helpers that route through `m.Update()`
- "Bypasses interaction" = tests that call internal methods (`startProjectForm`,
  `saveForm`, `exitForm`, `h.SubmitForm()`, etc.) directly, skipping the
  keypress dispatch layer

## Results

### Files using interaction helpers (24 files -- PASS)

These follow the testing rules. No action needed.

`bench_test.go`, `calendar_test.go`, `chat_test.go`,
`chat_coverage_test.go`, `column_finder_test.go`, `dashboard_test.go`,
`dashboard_load_test.go`, `demo_data_test.go`, `detail_test.go`,
`filter_test.go`, `form_filepicker_test.go`, `form_save_test.go`,
`handler_form_wiring_test.go` (partial), `inline_edit_dispatch_test.go`,
`inline_input_test.go`, `lazy_reload_test.go`, `mag_test.go`,
`mode_test.go`, `notes_test.go`, `overlay_status_test.go`, `sort_test.go`,
`undo_test.go`, `view_test.go`

### Test infrastructure (3 files -- no tests, PASS)

- `testmain_test.go` -- TestMain seed setup
- `model_with_store_test.go` -- helper function only
- `model_with_demo_data_test.go` -- helper function only

### Pure function / data-transformation tests (9 files -- legitimate supplements)

These test pure functions with no side effects and no user-facing dispatch
path. They are valid supplements under the testing rules.

| File | What it tests |
|---|---|
| `compact_test.go` | `formatInterval`, `statusLabel`, `compactMoneyValue`, `compactMoneyCells`, `annotateMoneyHeaders` |
| `form_validators_test.go` | Validator functions (`requiredText`, `optionalInt`, `optionalDate`, `endDateAfterStart`, etc.), form value converters (`projectFormValues`, `vendorFormValues`, etc.), `TestFormDataStructsHaveNoReferenceFields` |
| `dashboard_rows_test.go` | `dashMaintSplitRows`, `dashProjectRows`, `dashExpiringRows` |
| `docopen_test.go` | `wrapOpenerError`, `isDocumentTab` |
| `form_helpers_test.go` | `requiredDate`, `documentFormValues`, `documentEntityOptions`, `incidentFormValues`, `serviceLogFormValues`, `vendorFormValues` |
| `form_select_test.go` | `withOrdinals`, `selectOrdinal`, `isSelectField`, `selectOptionCount`, `jumpSelectToOrdinal` |
| `handlers_test.go` | Tab/handler wiring (`AllTabsHaveHandlers`, `HandlerForFormKind`, `HandlerFormKindMatchesTabKind`) |
| `table_style_test.go` | `warrantyStyleAt`, `urgencyStyleAt` |
| `rows_test.go` | Row-building functions (`projectRows`, `quoteRows`, `maintenanceRows`, `applianceRows`, `documentRows`, etc.), cell helpers (`centsCell`, `dateCell`, `nullTextCell`, `formatFileSize`) |

**Verdict**: Keep as-is. These are the "internal unit tests as supplements"
pattern the rules allow.

### Vendor tests (`vendor_test.go` -- mostly PASS)

Tests tab existence, column specs, row building, `vendorFormValues`, column
links, and `VendorHandlerDeleteRestore`. All are pure function / handler-API
tests. The Delete/Restore test calls the handler directly (legitimate API
test -- the user-facing delete flow is tested in `mode_test.go` via
`TestDeleteAutoShowsDeletedAndRestoreWorks`).

**Verdict**: Keep as-is.

### Flagged files -- bypass user interaction

#### 1. `lighter_forms_test.go` -- REWRITE 6 tests, KEEP 8

**Tests to rewrite** (test user-visible behavior via internal method calls):

| Test | What it does | Overlap with existing? |
|---|---|---|
| `TestSaveFormFocusesNewItem` | Creates items via `startProjectForm()`/`saveForm()`, checks cursor position | No interaction-level test covers cursor focus after save |
| `TestSaveFormInPlaceThenEscFocusesNewItem` | Tests ctrl+s then esc cursor position | No interaction-level test for this flow |
| `TestSaveFormInPlaceThenDiscardFocusesNewItem` | Tests ctrl+s then dirty-discard cursor | No interaction-level test |
| `TestSaveFormInPlaceTwiceThenEscFocusesItem` | Tests double ctrl+s then esc cursor | No interaction-level test |
| `TestEditExistingThenEscKeepsCursor` | Tests edit abort cursor position | No interaction-level test |
| `TestExitFormWithNoSaveNoCursorMove` | Tests aborting empty form cursor | No interaction-level test |

These 6 should be rewritten to use `openAddForm(m)` / `sendKey(m, "ctrl+s")`
/ `sendKey(m, "esc")` instead of calling internal methods directly.

**Tests to keep as-is** (structural/rendering checks, not behavioral flows):

| Test | Why keep |
|---|---|
| `TestAddProjectFormHasOnlyEssentialFields` | Checks form field visibility -- rendering test |
| `TestEditProjectFormHasMoreFieldsThanAdd` | Checks add vs edit field sets |
| `TestAddVendorFormHasOnlyName` | Field visibility |
| `TestEditVendorFormHasAllFields` | Field visibility |
| `TestAddApplianceFormHasOnlyName` | Field visibility |
| `TestAddMaintenanceFormHasOnlyEssentialFields` | Field visibility |
| `TestAddQuoteFormHasOnlyEssentialFields` | Field visibility |
| `TestAddServiceLogFormHasOnlyEssentialFields` | Field visibility |

These test what fields appear in the form UI, not a behavioral flow. They
can be converted to use `openAddForm` for setup, but the form-opening method
is the same either way. Low priority -- mark as supplement.

#### 2. `handler_crud_test.go` -- KEEP (handler-API tests)

This file tests the `TabHandler` interface methods directly:
`SubmitForm`, `Load`, `Delete`, `Restore`, `StartAddForm`, `StartEditForm`,
`InlineEdit`, `Snapshot`, `SyncFixedValues`.

These are legitimate API-level tests of the handler implementations. The
user-facing flows that exercise the same paths are already tested in:
- `form_save_test.go` (add/edit via keypresses)
- `mode_test.go` (delete/restore via keypresses)
- `inline_edit_dispatch_test.go` (inline edit via keypresses)

The handler_crud_test.go tests are supplements that verify handler behavior
independent of the dispatch layer. They catch handler-level bugs that
interaction tests might mask.

Also note: the file itself includes several interaction tests
(`TestMaintenanceInlineEditNextSetsDueDateAndSaves`,
`TestMaintenanceInlineEditEverySetIntervalAndSaves`,
`TestProjectStatusFilterToggleKeysReloadRows`,
`TestDeleteAutoShowsDeletedAndRestoreWorks`,
`TestSaveFormInPlaceSetEditID`).

**Verdict**: Keep as-is -- legitimate supplement.

#### 3. `handler_form_wiring_test.go` -- KEEP (API coverage tests)

Tests `StartAddForm`, `StartEditForm`, `InlineEdit` for every handler type
plus error paths (non-existent IDs). These verify handler wiring and error
handling. The user-facing equivalents are in `inline_edit_dispatch_test.go`
and `form_save_test.go`.

**Verdict**: Keep as-is -- legitimate supplement.

#### 4. `extraction_test.go` -- MINOR FIX

Uses `sendExtractionKey` which calls `m.handleExtractionKey(msg)` directly
instead of routing through `m.Update(msg)`. The tests construct real
`tea.KeyMsg` values but skip the main dispatch. This means they miss any
bugs in the Update-level routing to the extraction handler.

**Fix**: Change `sendExtractionKey` to route through `m.Update(msg)` instead
of calling `m.handleExtractionKey(msg)` directly (same pattern as `sendKey`).

## Action Plan

1. **Rewrite 6 tests in `lighter_forms_test.go`** to use `openAddForm` /
   `sendKey` instead of calling `startProjectForm()` / `saveForm()` /
   `exitForm()` directly
2. **Fix `extraction_test.go`** `sendExtractionKey` to route through
   `m.Update()` instead of `m.handleExtractionKey()`
3. **Keep everything else** as-is (legitimate pure-function supplements or
   handler-API tests)
