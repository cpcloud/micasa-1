<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Ops Tree Overlay (#766)

Explorable JSON tree overlay for the documents table's extraction operations
column.

## Design

- **New `cellOps` kind**: Renders like `cellDrilldown` (pill badge) but on
  Enter opens a tree overlay instead of drilling into a detail view.
- **New "Ops" column** in `documentColumnDefs`: shows operation count,
  right-aligned, between Model and Notes.
- **Tree overlay**: Self-contained component using existing overlay
  infrastructure (`overlay.Composite` + `cancelFaint`).
- **Slot**: Between note preview and column finder in the overlay stack
  (conceptually similar weight to note preview).

## Tree structure

Each top-level node = one `extract.Operation` (label: `action table`).
Children = sorted key-value pairs from `Data`.

```
▼ create vendors
  │ name: "Garcia Plumbing"
  │ email: "info@garcia.com"
  │ phone: "555-1234"
▶ update documents
```

## Navigation

- `j`/`k` — move cursor up/down through visible nodes
- `enter`/`l` — expand node (if collapsed)
- `h` — collapse node (if expanded)
- `esc`/`q` — dismiss overlay
- Mouse click on expand/collapse triangle via zone marks

## Implementation steps

1. Add `cellOps` to `cellKind` iota in `types.go`
2. Add "Ops" column to `documentColumnDefs` in `coldefs.go`
3. Run `go generate ./internal/app/`
4. Add ops cell to `documentRows()` in `tables.go`
5. Create `ops_tree.go`:
   - `opsTreeState` struct with ops, cursor, expanded map
   - `treeNode` struct for flat list of visible nodes
   - Key handler
   - Overlay renderer
6. Wire `cellOps` rendering in `table.go` (reuse pill rendering)
7. Wire `handleNormalEnter()` in `model.go` to open tree overlay on `cellOps`
8. Wire overlay into `buildView()`, `hasActiveOverlay()`, `dispatchOverlay()`,
   `dismissActiveOverlay()`
9. Add `enterHint()` for `cellOps`
10. Add zone marks for clickable expand/collapse
11. Write user-interaction tests
