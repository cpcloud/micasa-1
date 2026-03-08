<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Type-safe formData (issue #687)

## Problem

`formState.formData` is `any`. A mismatch between `formKind` and the stored
concrete type panics at runtime via `formDataAs[T]`. The compiler cannot catch
this class of bug.

## Design

Introduce a `formData` interface with a `formKind() FormKind` marker method.
Each form data struct implements it, returning its constant `FormKind`. This
gives compile-time safety: only valid form data types can be stored.

### Interface

```go
type formData interface {
    formKind() FormKind
}
```

### Key changes

1. **Remove `formKind` field from `formState`** -- replace with a
   `formKind() FormKind` method that derives from `formData` (returns
   `formNone` when nil). All reads `m.fs.formKind` become `m.fs.formKind()`.

2. **Change field types** from `any` to `formData` on `formState` (both
   `formData` and `formSnapshot`), `inlineInputState`, and `editorState`.

3. **Drop `kind FormKind` parameter** from `activateForm`, `openDatePicker`,
   `openInlineEdit`, `openNotesEdit`, `openInlineInput`, `openNotesTextarea`
   -- derive from `values.formKind()`.

4. **Remove `FormKind` field** from `inlineInputState` and `editorState` --
   derive from `FormData.formKind()`.

5. **Update `cloneFormData`** signature to `formData -> formData`.

### What stays the same

- `formDataAs[T any]` generic function -- still works, asserts interface to
  concrete pointer type.
- `handleFormSubmit` dispatch logic -- reads `m.fs.formKind()` instead of
  field.
- `TabHandler.FormKind()` -- exported method on handlers, unchanged.
