<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Architectural Refactoring

Tracked in #568.

## Motivation

`Model` has 56 fields and ~330 methods -- a God object. The form, extraction,
and data layers contain significant boilerplate from repeated per-entity
scaffolding. This plan addresses both concerns in 8 incremental commits.

## Items

### 1. Extract `formState` from `Model`

Move 13 form-related fields into a `formState` struct held as a value field on
`Model`. Methods that only touch form state become methods on `*formState`;
methods that also need `Model` stay on `Model` but access `m.fs.*` instead of
`m.*`. No logic changes -- purely structural.

Fields: `formKind`, `form`, `formData`, `formSnapshot`, `formDirty`,
`confirmDiscard`, `confirmQuit`, `formHasRequired`, `pendingFormInit`,
`editID`, `notesEditMode`, `notesFieldPtr`, `pendingEditor`.

### 2. Extract `extractionState` from `Model`

Same pattern for 10 extraction-related fields: `extractionModel`,
`extractionEnabled`, `extractionThinking`, `extractionClient`, `extractors`,
`extractionReady`, `pendingExtractionDocID`, `extraction`, `bgExtractions`,
`pull`.

### 3. `unscopedPreload` in data layer

Replace ~12 identical `func(q *gorm.DB) *gorm.DB { return q.Unscoped() }`
lambdas with a package-level var.

### 4. Generic `optionSlice[T]`

Replace 4 near-identical option builder functions with a single generic.

### 5. Table-driven `submitForm`

Replace 8 identical submit method bodies with a shared helper.

### 6. Table-driven inline edit dispatch

Replace 7 switch statements in `inlineEdit*` with a map lookup.

### 7. Generic CRUD in data layer

Introduce entity configs and generic List/Get/Create/Update/Delete/Restore
functions for the 8 soft-deletable entities.

### 8. Overlay interface

Unify the 7 overlay guard blocks behind an `Overlay` interface.
