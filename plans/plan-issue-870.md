<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Plan: Set exhaustive default-signifies-exhaustive to false (#870)

## Summary

Change `.golangci.yml` `exhaustive.default-signifies-exhaustive` from `true`
to `false` so that every enum value requires an explicit `case`, catching
missing branches at lint time instead of hiding them behind `default:`.

## Violations (18 total)

Running the linter with the new setting surfaces 18 violations across 3
categories:

### A. Replace `default:` with explicit cases (our own enums - 14 sites)

| File | Line | Enum | Missing cases |
|------|------|------|---------------|
| mag.go | 31 | cellKind | cellText, cellReadonly, cellDate, cellStatus, cellWarranty, cellUrgency, cellNotes, cellEntity |
| mag_test.go | 297 | cellKind | cellText, cellDate, cellStatus, cellDrilldown, cellWarranty, cellUrgency, cellNotes, cellEntity, cellOps |
| mag_test.go | 384 | cellKind | (same as above) |
| model.go | 1107 | FormKind | formNone, formProject, formQuote, formMaintenance, formAppliance, formIncident, formServiceLog, formDocument |
| model_status.go | 138 | TabKind | tabProjects, tabQuotes, tabAppliances, tabVendors, tabDocuments |
| ops_tree.go | 606 | treeValueKind | tvOther |
| sort.go | 114 | cellKind | cellText, cellStatus, cellNotes, cellEntity |
| sync.go | 86 | syncStatus | syncIdle |
| table.go | 641 | cellKind | cellText, cellDate, cellStatus, cellDrilldown, cellWarranty, cellUrgency, cellNotes, cellEntity, cellOps |
| table.go | 774 | alignKind | alignLeft |
| dashboard.go | 31 | time.Month | December, January, February |
| units.go | 83 | UnitSystem | UnitsImperial |
| units.go | 98 | UnitSystem | UnitsImperial |
| units.go | 109 | UnitSystem | UnitsImperial |

**Strategy**: Replace `default:` with the explicit list of remaining values.
When a new enum value is added, the linter will flag it.

### B. `//nolint:exhaustive` (stdlib/third-party enums - 4 sites)

| File | Line | Enum | Reason |
|------|------|------|--------|
| model_update.go | 356 | huh.FormState | Third-party enum (charmbracelet/huh) |
| config.go | 434 | reflect.Kind | stdlib enum with 26 values; only config-relevant kinds handled |
| config.go | 593 | reflect.Kind | stdlib enum with 26 values |
| validate.go | 58 | reflect.Kind | stdlib enum with 26 values |

**Strategy**: Add `//nolint:exhaustive` with a justification comment.

## Risks

- None significant. All existing `default:` behavior is preserved; we're just
  making the case list explicit so the linter catches future omissions.
