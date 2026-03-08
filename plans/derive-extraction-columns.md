<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Derive ExtractionTableDefs columns from genmeta

Issue: #666

## Problem

`ExtractionTableDefs` in `internal/extract/sqlcontext.go` manually lists column
names and types for each extractable table. This duplicates information already
available from the GORM model structs that `genmeta` processes. Adding or
renaming a model field requires updating both `models.go` and
`ExtractionTableDefs`, and they can drift silently.

## Design

### 1. Extend genmeta to emit per-table column metadata

Add a new type and map to `meta_generated.go`:

```go
type MetaColumn struct {
    Name     string
    JSONType string // "string" or "integer"
}

var TableExtractColumns = map[string][]MetaColumn{...}
```

**Column inclusion rules** (applied in genmeta AST walk):
- Exclude `ID`, `CreatedAt`, `UpdatedAt`, `DeletedAt` fields (infrastructure)
- Exclude `[]byte` fields (binary data)
- Exclude `gorm.DeletedAt` type (already caught by name, belt-and-suspenders)
- Exclude associations (already excluded by `isAssociation`)
- Exclude `gorm:"-"` tagged fields (already excluded)

**JSON Schema type mapping** (from AST type expressions):
- `string` -> `"string"`
- `int`, `int64`, `uint`, `float64` and `*` variants -> `"integer"`
- `time.Time`, `*time.Time` -> `"string"` (dates are strings for the LLM)

### 2. Add table-level Omit to TableDef

Currently `Omit` only exists on `ActionDef`. Add it to `TableDef` too so
generated columns that aren't yet handled by commit functions can be excluded
from all actions without repeating the list:

```go
type TableDef struct {
    Table   string
    Columns []ColumnDef
    Omit    []string    // columns to exclude from ALL actions
    Actions []ActionDef
}
```

Update `expandTableOp` to merge both `td.Omit` and `ad.Omit`.

### 3. Refactor ExtractionTableDefs

Replace manual column lists with a helper that reads generated metadata:

```go
func columnsFromMeta(table string) []ColumnDef {
    metas := data.TableExtractColumns[table]
    cols := make([]ColumnDef, len(metas))
    for i, m := range metas {
        cols[i] = ColumnDef{Name: m.Name, Type: ColType(m.JSONType)}
    }
    return cols
}
```

Plus helpers for enum overrides and synthetic columns:

```go
func withEnum(cols []ColumnDef, name string, values []any) []ColumnDef
func withSynthetic(cols []ColumnDef, extra ...ColumnDef) []ColumnDef
```

Each table definition becomes generated columns + policy annotations. Example:

```go
{
    Table:   data.TableVendors,
    Columns: columnsFromMeta(data.TableVendors),
    Actions: []ActionDef{
        {Action: ActionCreate, Required: []string{"name"}},
        {Action: ActionUpdate, Required: []string{"id"}, Extra: []ColumnDef{
            {Name: "id", Type: ColTypeInteger},
        }},
    },
},
```

### 4. Table-by-table Omit plan (behavioral parity)

Generated columns not currently exposed to the LLM are omitted to preserve
identical behavior. Each can be un-omitted later as commit functions are
updated to handle them.

| Table | Generated but omitted |
|---|---|
| vendors | (none) |
| appliances | purchase_date, warranty_expiry |
| projects | start_date, end_date, actual_cents |
| quotes | other_cents, received_date |
| maintenance_items | last_serviced_at, due_date, manual_url, manual_text |
| incidents | previous_status, date_resolved |
| service_log_entries | (none) |
| documents | mime_type, size_bytes, sha256, extracted_text |

### 5. Consistency test

Add a test that verifies every non-omitted, non-Extra column in each
`ExtractionTableDefs` entry exists in `TableExtractColumns[table]` (or is
marked synthetic). This catches future drift between models and extraction
config.

## Files changed

- `internal/data/cmd/genmeta/main.go` - emit `MetaColumn` type + `TableExtractColumns` map
- `internal/data/meta_generated.go` - regenerated output
- `internal/extract/sqlcontext.go` - refactored `ExtractionTableDefs`, new helpers
- `internal/extract/sqlcontext_test.go` - consistency test

## Non-goals

- Changing which columns the LLM can write (behavioral parity)
- Updating commit functions to handle newly-available columns (future work)
- Generating Actions/Required/Enum/Omit annotations (these stay manual)
