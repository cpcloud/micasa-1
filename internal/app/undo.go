// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
)

const maxUndoStack = 50

// undoEntry represents a reversible edit. FormKind and EntityID identify the
// entity so the opposite stack can snapshot the current state before restoring.
type undoEntry struct {
	Description string
	FormKind    FormKind
	EntityID    uint // 0 for house profile (singleton)
	Restore     func() error
}

// snapshotForUndo loads the current entity from the DB and pushes an undo
// entry. Only applies to edits (editID != nil or house form), not creates.
// Clears the redo stack since a new forward edit invalidates redo history.
func (m *Model) snapshotForUndo() {
	if m.fs.formKind == formHouse {
		if !m.hasHouse {
			return
		}
		entry, ok := m.snapshotEntity(formHouse, 0)
		if ok {
			m.pushUndo(entry)
			m.redoStack = nil
		}
		return
	}

	if m.fs.editID == nil {
		return
	}
	entry, ok := m.snapshotEntity(m.fs.formKind, *m.fs.editID)
	if ok {
		m.pushUndo(entry)
		m.redoStack = nil
	}
}

// snapshotEntity captures the current DB state of an entity and returns an
// undoEntry that can restore it. The house form is handled as a special case;
// all other entity types delegate to their TabHandler.
func (m *Model) snapshotEntity(kind FormKind, id uint) (undoEntry, bool) {
	if kind == formHouse {
		if !m.hasHouse {
			return undoEntry{}, false
		}
		old := m.house
		return undoEntry{
			Description: fmt.Sprintf("house %q", old.Nickname),
			FormKind:    formHouse,
			EntityID:    0,
			Restore: func() error {
				return m.store.UpdateHouseProfile(old)
			},
		}, true
	}
	handler := m.handlerForFormKind(kind)
	if handler == nil {
		return undoEntry{}, false
	}
	return handler.Snapshot(m.store, id)
}

func (m *Model) pushUndo(entry undoEntry) {
	m.undoStack = append(m.undoStack, entry)
	if len(m.undoStack) > maxUndoStack {
		m.undoStack = m.undoStack[len(m.undoStack)-maxUndoStack:]
	}
}

func (m *Model) pushRedo(entry undoEntry) {
	m.redoStack = append(m.redoStack, entry)
	if len(m.redoStack) > maxUndoStack {
		m.redoStack = m.redoStack[len(m.redoStack)-maxUndoStack:]
	}
}

// popUndo restores the previous state. Before restoring, it snapshots the
// current state and pushes it onto the redo stack.
func (m *Model) popUndo() error {
	if len(m.undoStack) == 0 {
		return fmt.Errorf("nothing to undo")
	}
	entry := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]

	// Snapshot current state for redo before restoring.
	if m.store != nil {
		if redo, ok := m.snapshotEntity(entry.FormKind, entry.EntityID); ok {
			m.pushRedo(redo)
		}
	}

	if err := entry.Restore(); err != nil {
		return fmt.Errorf("undo %s: %w", entry.Description, err)
	}

	m.setStatusInfo(fmt.Sprintf("Undone: %s", entry.Description))
	return nil
}

// popRedo re-applies the last undone change. Before re-applying, it snapshots
// the current state and pushes it onto the undo stack.
func (m *Model) popRedo() error {
	if len(m.redoStack) == 0 {
		return fmt.Errorf("nothing to redo")
	}
	entry := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]

	// Snapshot current state for undo before re-applying.
	if m.store != nil {
		if undo, ok := m.snapshotEntity(entry.FormKind, entry.EntityID); ok {
			m.pushUndo(undo)
		}
	}

	if err := entry.Restore(); err != nil {
		return fmt.Errorf("redo %s: %w", entry.Description, err)
	}

	m.setStatusInfo(fmt.Sprintf("Redone: %s", entry.Description))
	return nil
}
