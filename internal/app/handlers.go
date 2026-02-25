// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/cpcloud/micasa/internal/data"
)

// TabHandler encapsulates entity-specific operations for a tab, eliminating
// TabKind/FormKind switch dispatch scattered across the codebase. Each entity
// type (projects, quotes, maintenance, appliances) implements this interface.
type TabHandler interface {
	// FormKind returns the FormKind that identifies this entity in forms and
	// undo entries.
	FormKind() FormKind

	// Load fetches entities and converts them to table rows.
	Load(store *data.Store, showDeleted bool) ([]table.Row, []rowMeta, [][]cell, error)

	// Delete soft-deletes the entity with the given ID.
	Delete(store *data.Store, id uint) error

	// Restore reverses a soft-delete.
	Restore(store *data.Store, id uint) error

	// StartAddForm opens a "new entity" form on the model.
	StartAddForm(m *Model) error

	// StartEditForm opens an "edit entity" form for the given ID.
	StartEditForm(m *Model, id uint) error

	// InlineEdit opens a single-field editor for the given column.
	InlineEdit(m *Model, id uint, col int) error

	// SubmitForm persists the current form data (create or update).
	SubmitForm(m *Model) error

	// Snapshot captures the current DB state of an entity for undo/redo.
	Snapshot(store *data.Store, id uint) (undoEntry, bool)

	// SyncFixedValues updates column specs with values from dynamic lookup
	// tables so column widths stay stable.
	SyncFixedValues(m *Model, specs []columnSpec)
}

// handlerForFormKind finds the tab handler that owns the given FormKind.
// Checks both main tabs and the detail tab (if active).
// Returns nil for formHouse (no tab) or unknown kinds.
func (m *Model) handlerForFormKind(kind FormKind) TabHandler {
	// Check the detail tab first since it may shadow a main tab's form kind.
	if dc := m.detail(); dc != nil && dc.Tab.Handler != nil &&
		dc.Tab.Handler.FormKind() == kind {
		return dc.Tab.Handler
	}
	for i := range m.tabs {
		if m.tabs[i].Handler != nil && m.tabs[i].Handler.FormKind() == kind {
			return m.tabs[i].Handler
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// projectHandler
// ---------------------------------------------------------------------------

type projectHandler struct{}

func (projectHandler) FormKind() FormKind { return formProject }

func (projectHandler) Load(
	store *data.Store,
	showDeleted bool,
) ([]table.Row, []rowMeta, [][]cell, error) {
	projects, err := store.ListProjects(showDeleted)
	if err != nil {
		return nil, nil, nil, err
	}
	ids := entityIDs(projects, func(p data.Project) uint { return p.ID })
	// Supplementary counts degrade to 0 on error so the primary entity
	// list still renders. This pattern repeats across all handlers.
	quoteCounts, err := store.CountQuotesByProject(ids)
	if err != nil {
		quoteCounts = map[uint]int{}
	}
	docCounts, err := store.CountDocumentsByEntity(data.DocumentEntityProject, ids)
	if err != nil {
		docCounts = map[uint]int{}
	}
	rows, meta, cellRows := projectRows(projects, quoteCounts, docCounts)
	return rows, meta, cellRows, nil
}

func (projectHandler) Delete(store *data.Store, id uint) error {
	return store.DeleteProject(id)
}

func (projectHandler) Restore(store *data.Store, id uint) error {
	return store.RestoreProject(id)
}

func (projectHandler) StartAddForm(m *Model) error {
	m.startProjectForm()
	return nil
}

func (projectHandler) StartEditForm(m *Model, id uint) error {
	return m.startEditProjectForm(id)
}

func (projectHandler) InlineEdit(m *Model, id uint, col int) error {
	return m.inlineEditProject(id, projectCol(col))
}

func (projectHandler) SubmitForm(m *Model) error {
	return m.submitProjectForm()
}

func (projectHandler) Snapshot(store *data.Store, id uint) (undoEntry, bool) {
	project, err := store.GetProject(id)
	if err != nil {
		return undoEntry{}, false
	}
	return undoEntry{
		Description: fmt.Sprintf("project %q", project.Title),
		FormKind:    formProject,
		EntityID:    id,
		Restore: func() error {
			return store.UpdateProject(project)
		},
	}, true
}

func (projectHandler) SyncFixedValues(m *Model, specs []columnSpec) {
	typeNames := make([]string, len(m.projectTypes))
	for i, pt := range m.projectTypes {
		typeNames[i] = pt.Name
	}
	setFixedValues(specs, "Type", typeNames)
}

// ---------------------------------------------------------------------------
// quoteHandler
// ---------------------------------------------------------------------------

type quoteHandler struct{}

func (quoteHandler) FormKind() FormKind { return formQuote }

func (quoteHandler) Load(
	store *data.Store,
	showDeleted bool,
) ([]table.Row, []rowMeta, [][]cell, error) {
	quotes, err := store.ListQuotes(showDeleted)
	if err != nil {
		return nil, nil, nil, err
	}
	ids := entityIDs(quotes, func(q data.Quote) uint { return q.ID })
	docCounts, err := store.CountDocumentsByEntity(data.DocumentEntityQuote, ids)
	if err != nil {
		docCounts = map[uint]int{}
	}
	rows, meta, cellRows := quoteRows(quotes, docCounts)
	return rows, meta, cellRows, nil
}

func (quoteHandler) Delete(store *data.Store, id uint) error {
	return store.DeleteQuote(id)
}

func (quoteHandler) Restore(store *data.Store, id uint) error {
	return store.RestoreQuote(id)
}

func (quoteHandler) StartAddForm(m *Model) error {
	return m.startQuoteForm()
}

func (quoteHandler) StartEditForm(m *Model, id uint) error {
	return m.startEditQuoteForm(id)
}

func (quoteHandler) InlineEdit(m *Model, id uint, col int) error {
	return m.inlineEditQuote(id, quoteCol(col))
}

func (quoteHandler) SubmitForm(m *Model) error {
	return m.submitQuoteForm()
}

func (quoteHandler) Snapshot(store *data.Store, id uint) (undoEntry, bool) {
	quote, err := store.GetQuote(id)
	if err != nil {
		return undoEntry{}, false
	}
	vendor := quote.Vendor
	return undoEntry{
		Description: fmt.Sprintf("quote from %s", vendor.Name),
		FormKind:    formQuote,
		EntityID:    id,
		Restore: func() error {
			return store.UpdateQuote(quote, vendor)
		},
	}, true
}

func (quoteHandler) SyncFixedValues(_ *Model, _ []columnSpec) {}

// ---------------------------------------------------------------------------
// maintenanceHandler
// ---------------------------------------------------------------------------

type maintenanceHandler struct{}

func (maintenanceHandler) FormKind() FormKind { return formMaintenance }

func (maintenanceHandler) Load(
	store *data.Store,
	showDeleted bool,
) ([]table.Row, []rowMeta, [][]cell, error) {
	items, err := store.ListMaintenance(showDeleted)
	if err != nil {
		return nil, nil, nil, err
	}
	// Batch-fetch service log counts and document counts for all items.
	ids := entityIDs(items, func(item data.MaintenanceItem) uint { return item.ID })
	logCounts, err := store.CountServiceLogs(ids)
	if err != nil {
		logCounts = map[uint]int{} // non-fatal
	}
	docCounts, err := store.CountDocumentsByEntity(data.DocumentEntityMaintenance, ids)
	if err != nil {
		docCounts = map[uint]int{}
	}
	rows, meta, cellRows := maintenanceRows(items, logCounts, docCounts)
	return rows, meta, cellRows, nil
}

func (maintenanceHandler) Delete(store *data.Store, id uint) error {
	return store.DeleteMaintenance(id)
}

func (maintenanceHandler) Restore(store *data.Store, id uint) error {
	return store.RestoreMaintenance(id)
}

func (maintenanceHandler) StartAddForm(m *Model) error {
	return m.startMaintenanceForm()
}

func (maintenanceHandler) StartEditForm(m *Model, id uint) error {
	return m.startEditMaintenanceForm(id)
}

func (maintenanceHandler) InlineEdit(m *Model, id uint, col int) error {
	return m.inlineEditMaintenance(id, maintenanceCol(col))
}

func (maintenanceHandler) SubmitForm(m *Model) error {
	return m.submitMaintenanceForm()
}

func (maintenanceHandler) Snapshot(store *data.Store, id uint) (undoEntry, bool) {
	item, err := store.GetMaintenance(id)
	if err != nil {
		return undoEntry{}, false
	}
	return undoEntry{
		Description: fmt.Sprintf("maintenance %q", item.Name),
		FormKind:    formMaintenance,
		EntityID:    id,
		Restore: func() error {
			return store.UpdateMaintenance(item)
		},
	}, true
}

func (maintenanceHandler) SyncFixedValues(m *Model, specs []columnSpec) {
	catNames := make([]string, len(m.maintenanceCategories))
	for i, c := range m.maintenanceCategories {
		catNames[i] = c.Name
	}
	setFixedValues(specs, "Category", catNames)
}

// ---------------------------------------------------------------------------
// applianceHandler
// ---------------------------------------------------------------------------

type applianceHandler struct{}

func (applianceHandler) FormKind() FormKind { return formAppliance }

func (applianceHandler) Load(
	store *data.Store,
	showDeleted bool,
) ([]table.Row, []rowMeta, [][]cell, error) {
	items, err := store.ListAppliances(showDeleted)
	if err != nil {
		return nil, nil, nil, err
	}
	ids := entityIDs(items, func(a data.Appliance) uint { return a.ID })
	maintCounts, err := store.CountMaintenanceByAppliance(ids)
	if err != nil {
		maintCounts = map[uint]int{}
	}
	docCounts, err := store.CountDocumentsByEntity(data.DocumentEntityAppliance, ids)
	if err != nil {
		docCounts = map[uint]int{}
	}
	rows, meta, cellRows := applianceRows(items, maintCounts, docCounts, time.Now())
	return rows, meta, cellRows, nil
}

func (applianceHandler) Delete(store *data.Store, id uint) error {
	return store.DeleteAppliance(id)
}

func (applianceHandler) Restore(store *data.Store, id uint) error {
	return store.RestoreAppliance(id)
}

func (applianceHandler) StartAddForm(m *Model) error {
	m.startApplianceForm()
	return nil
}

func (applianceHandler) StartEditForm(m *Model, id uint) error {
	return m.startEditApplianceForm(id)
}

func (applianceHandler) InlineEdit(m *Model, id uint, col int) error {
	return m.inlineEditAppliance(id, applianceCol(col))
}

func (applianceHandler) SubmitForm(m *Model) error {
	return m.submitApplianceForm()
}

func (applianceHandler) Snapshot(store *data.Store, id uint) (undoEntry, bool) {
	item, err := store.GetAppliance(id)
	if err != nil {
		return undoEntry{}, false
	}
	return undoEntry{
		Description: fmt.Sprintf("appliance %q", item.Name),
		FormKind:    formAppliance,
		EntityID:    id,
		Restore: func() error {
			return store.UpdateAppliance(item)
		},
	}, true
}

func (applianceHandler) SyncFixedValues(_ *Model, _ []columnSpec) {}

// ---------------------------------------------------------------------------
// incidentHandler
// ---------------------------------------------------------------------------

type incidentHandler struct{}

func (incidentHandler) FormKind() FormKind { return formIncident }

func (incidentHandler) Load(
	store *data.Store,
	showDeleted bool,
) ([]table.Row, []rowMeta, [][]cell, error) {
	items, err := store.ListIncidents(showDeleted)
	if err != nil {
		return nil, nil, nil, err
	}
	ids := entityIDs(items, func(inc data.Incident) uint { return inc.ID })
	docCounts, err := store.CountDocumentsByEntity(data.DocumentEntityIncident, ids)
	if err != nil {
		docCounts = map[uint]int{}
	}
	rows, meta, cellRows := incidentRows(items, docCounts)
	return rows, meta, cellRows, nil
}

func (incidentHandler) Delete(store *data.Store, id uint) error {
	return store.DeleteIncident(id)
}

func (incidentHandler) Restore(store *data.Store, id uint) error {
	return store.RestoreIncident(id)
}

func (incidentHandler) StartAddForm(m *Model) error {
	return m.startIncidentForm()
}

func (incidentHandler) StartEditForm(m *Model, id uint) error {
	return m.startEditIncidentForm(id)
}

func (incidentHandler) InlineEdit(m *Model, id uint, col int) error {
	return m.inlineEditIncident(id, incidentCol(col))
}

func (incidentHandler) SubmitForm(m *Model) error {
	return m.submitIncidentForm()
}

func (incidentHandler) Snapshot(store *data.Store, id uint) (undoEntry, bool) {
	item, err := store.GetIncident(id)
	if err != nil {
		return undoEntry{}, false
	}
	return undoEntry{
		Description: fmt.Sprintf("incident %q", item.Title),
		FormKind:    formIncident,
		EntityID:    id,
		Restore: func() error {
			return store.UpdateIncident(item)
		},
	}, true
}

func (incidentHandler) SyncFixedValues(_ *Model, specs []columnSpec) {
	setFixedValues(specs, "Status", []string{
		data.IncidentStatusOpen,
		data.IncidentStatusInProgress,
	})
	setFixedValues(specs, "Severity", []string{
		data.IncidentSeverityUrgent,
		data.IncidentSeveritySoon,
		data.IncidentSeverityWhenever,
	})
}

// ---------------------------------------------------------------------------
// scopedHandler wraps a parent TabHandler for detail-view sub-tables.
// The embedded TabHandler provides default implementations for all interface
// methods; only Load, InlineEdit, StartAddForm, and SubmitForm are overridden
// when the scoped view differs from the parent.
// ---------------------------------------------------------------------------

type scopedHandler struct {
	TabHandler   // embedded; delegates FormKind, Delete, Restore, StartEditForm, Snapshot, SyncFixedValues
	loadFn       func(*data.Store, bool) ([]table.Row, []rowMeta, [][]cell, error)
	inlineEditFn func(*Model, uint, int) error // nil = TabHandler.InlineEdit
	startAddFn   func(*Model) error            // nil = TabHandler.StartAddForm
	submitFn     func(*Model) error            // nil = TabHandler.SubmitForm
}

func (s scopedHandler) Load(
	store *data.Store,
	showDeleted bool,
) ([]table.Row, []rowMeta, [][]cell, error) {
	return s.loadFn(store, showDeleted)
}

func (s scopedHandler) StartAddForm(m *Model) error {
	if s.startAddFn != nil {
		return s.startAddFn(m)
	}
	return s.TabHandler.StartAddForm(m)
}

func (s scopedHandler) InlineEdit(m *Model, id uint, col int) error {
	if s.inlineEditFn != nil {
		return s.inlineEditFn(m, id, col)
	}
	return s.TabHandler.InlineEdit(m, id, col)
}

func (s scopedHandler) SubmitForm(m *Model) error {
	if s.submitFn != nil {
		return s.submitFn(m)
	}
	return s.TabHandler.SubmitForm(m)
}

// skipColEdit returns an InlineEdit function that skips a removed column by
// remapping indices at and above skipAt to skipAt+1.
func skipColEdit(parent TabHandler, skipAt int) func(*Model, uint, int) error {
	return func(m *Model, id uint, col int) error {
		fullCol := col
		if col >= skipAt {
			fullCol = col + 1
		}
		return parent.InlineEdit(m, id, fullCol)
	}
}

// ---------------------------------------------------------------------------
// Scoped handler constructors
// ---------------------------------------------------------------------------

func newApplianceMaintenanceHandler(applianceID uint) scopedHandler {
	parent := maintenanceHandler{}
	return scopedHandler{
		TabHandler: parent,
		loadFn: func(store *data.Store, showDeleted bool) ([]table.Row, []rowMeta, [][]cell, error) {
			items, err := store.ListMaintenanceByAppliance(applianceID, showDeleted)
			if err != nil {
				return nil, nil, nil, err
			}
			ids := entityIDs(items, func(item data.MaintenanceItem) uint { return item.ID })
			logCounts, err := store.CountServiceLogs(ids)
			if err != nil {
				logCounts = map[uint]int{}
			}
			docCounts, err := store.CountDocumentsByEntity(data.DocumentEntityMaintenance, ids)
			if err != nil {
				docCounts = map[uint]int{}
			}
			rows, meta, cellRows := applianceMaintenanceRows(items, logCounts, docCounts)
			return rows, meta, cellRows, nil
		},
		inlineEditFn: skipColEdit(parent, 3), // skip Appliance column
	}
}

// ---------------------------------------------------------------------------
// serviceLogHandler -- detail-view handler for service log entries scoped to
// a single maintenance item.
// ---------------------------------------------------------------------------

type serviceLogHandler struct {
	maintenanceItemID uint
}

func (h serviceLogHandler) FormKind() FormKind { return formServiceLog }

func (h serviceLogHandler) Load(
	store *data.Store,
	showDeleted bool,
) ([]table.Row, []rowMeta, [][]cell, error) {
	entries, err := store.ListServiceLog(h.maintenanceItemID, showDeleted)
	if err != nil {
		return nil, nil, nil, err
	}
	ids := entityIDs(entries, func(e data.ServiceLogEntry) uint { return e.ID })
	docCounts, err := store.CountDocumentsByEntity(data.DocumentEntityServiceLog, ids)
	if err != nil {
		docCounts = map[uint]int{}
	}
	rows, meta, cellRows := serviceLogRows(entries, docCounts)
	return rows, meta, cellRows, nil
}

func (h serviceLogHandler) Delete(store *data.Store, id uint) error {
	return store.DeleteServiceLog(id)
}

func (h serviceLogHandler) Restore(store *data.Store, id uint) error {
	return store.RestoreServiceLog(id)
}

func (h serviceLogHandler) StartAddForm(m *Model) error {
	return m.startServiceLogForm(h.maintenanceItemID)
}

func (h serviceLogHandler) StartEditForm(m *Model, id uint) error {
	return m.startEditServiceLogForm(id)
}

func (h serviceLogHandler) InlineEdit(m *Model, id uint, col int) error {
	return m.inlineEditServiceLog(id, serviceLogCol(col))
}

func (h serviceLogHandler) SubmitForm(m *Model) error {
	return m.submitServiceLogForm()
}

func (h serviceLogHandler) Snapshot(store *data.Store, id uint) (undoEntry, bool) {
	entry, err := store.GetServiceLog(id)
	if err != nil {
		return undoEntry{}, false
	}
	vendor := entry.Vendor
	return undoEntry{
		Description: fmt.Sprintf("service log %s", entry.ServicedAt.Format("2006-01-02")),
		FormKind:    formServiceLog,
		EntityID:    id,
		Restore: func() error {
			return store.UpdateServiceLog(entry, vendor)
		},
	}, true
}

func (serviceLogHandler) SyncFixedValues(_ *Model, _ []columnSpec) {}

// ---------------------------------------------------------------------------
// vendorHandler
// ---------------------------------------------------------------------------

type vendorHandler struct{}

func (vendorHandler) FormKind() FormKind { return formVendor }

func (vendorHandler) Load(
	store *data.Store,
	showDeleted bool,
) ([]table.Row, []rowMeta, [][]cell, error) {
	vendors, err := store.ListVendors(showDeleted)
	if err != nil {
		return nil, nil, nil, err
	}
	ids := entityIDs(vendors, func(v data.Vendor) uint { return v.ID })
	quoteCounts, err := store.CountQuotesByVendor(ids)
	if err != nil {
		quoteCounts = map[uint]int{}
	}
	jobCounts, err := store.CountServiceLogsByVendor(ids)
	if err != nil {
		jobCounts = map[uint]int{}
	}
	docCounts, err := store.CountDocumentsByEntity(data.DocumentEntityVendor, ids)
	if err != nil {
		docCounts = map[uint]int{}
	}
	rows, meta, cellRows := vendorRows(vendors, quoteCounts, jobCounts, docCounts)
	return rows, meta, cellRows, nil
}

func (vendorHandler) Delete(store *data.Store, id uint) error {
	return store.DeleteVendor(id)
}

func (vendorHandler) Restore(store *data.Store, id uint) error {
	return store.RestoreVendor(id)
}

func (vendorHandler) StartAddForm(m *Model) error {
	m.startVendorForm()
	return nil
}

func (vendorHandler) StartEditForm(m *Model, id uint) error {
	return m.startEditVendorForm(id)
}

func (vendorHandler) InlineEdit(m *Model, id uint, col int) error {
	return m.inlineEditVendor(id, vendorCol(col))
}

func (vendorHandler) SubmitForm(m *Model) error {
	return m.submitVendorForm()
}

func (vendorHandler) Snapshot(store *data.Store, id uint) (undoEntry, bool) {
	vendor, err := store.GetVendor(id)
	if err != nil {
		return undoEntry{}, false
	}
	return undoEntry{
		Description: fmt.Sprintf("vendor %q", vendor.Name),
		FormKind:    formVendor,
		EntityID:    id,
		Restore: func() error {
			return store.UpdateVendor(vendor)
		},
	}, true
}

func (vendorHandler) SyncFixedValues(_ *Model, _ []columnSpec) {}

func newVendorQuoteHandler(vendorID uint) scopedHandler {
	parent := quoteHandler{}
	return scopedHandler{
		TabHandler: parent,
		loadFn: func(store *data.Store, showDeleted bool) ([]table.Row, []rowMeta, [][]cell, error) {
			quotes, err := store.ListQuotesByVendor(vendorID, showDeleted)
			if err != nil {
				return nil, nil, nil, err
			}
			ids := entityIDs(quotes, func(q data.Quote) uint { return q.ID })
			docCounts, err := store.CountDocumentsByEntity(data.DocumentEntityQuote, ids)
			if err != nil {
				docCounts = map[uint]int{}
			}
			rows, meta, cellRows := vendorQuoteRows(quotes, docCounts)
			return rows, meta, cellRows, nil
		},
		inlineEditFn: skipColEdit(parent, 2), // skip Vendor column
	}
}

func newVendorJobsHandler(vendorID uint) scopedHandler {
	parent := serviceLogHandler{}
	return scopedHandler{
		TabHandler: parent,
		loadFn: func(store *data.Store, showDeleted bool) ([]table.Row, []rowMeta, [][]cell, error) {
			entries, err := store.ListServiceLogsByVendor(vendorID, showDeleted)
			if err != nil {
				return nil, nil, nil, err
			}
			rows, meta, cellRows := vendorJobsRows(entries)
			return rows, meta, cellRows, nil
		},
		inlineEditFn: func(m *Model, id uint, col int) error {
			switch vendorJobsCol(col) {
			case vendorJobsColItem:
				m.setStatusInfo("Edit item from the Maintenance tab.")
				return nil
			case vendorJobsColDate:
				return m.inlineEditServiceLog(id, serviceLogColDate)
			case vendorJobsColCost:
				return m.inlineEditServiceLog(id, serviceLogColCost)
			case vendorJobsColNotes:
				return m.inlineEditServiceLog(id, serviceLogColNotes)
			case vendorJobsColID:
				return nil
			}
			return nil
		},
		startAddFn: func(_ *Model) error {
			return fmt.Errorf("add service log entries from the Maintenance tab")
		},
	}
}

func newProjectQuoteHandler(projectID uint) scopedHandler {
	parent := quoteHandler{}
	return scopedHandler{
		TabHandler: parent,
		loadFn: func(store *data.Store, showDeleted bool) ([]table.Row, []rowMeta, [][]cell, error) {
			quotes, err := store.ListQuotesByProject(projectID, showDeleted)
			if err != nil {
				return nil, nil, nil, err
			}
			ids := entityIDs(quotes, func(q data.Quote) uint { return q.ID })
			docCounts, err := store.CountDocumentsByEntity(data.DocumentEntityQuote, ids)
			if err != nil {
				docCounts = map[uint]int{}
			}
			rows, meta, cellRows := projectQuoteRows(quotes, docCounts)
			return rows, meta, cellRows, nil
		},
		inlineEditFn: skipColEdit(parent, 1), // skip Project column
	}
}

// ---------------------------------------------------------------------------
// documentHandler -- top-level handler for the Documents tab.
// ---------------------------------------------------------------------------

type documentHandler struct{}

func (documentHandler) FormKind() FormKind { return formDocument }

func (documentHandler) Load(
	store *data.Store,
	showDeleted bool,
) ([]table.Row, []rowMeta, [][]cell, error) {
	docs, err := store.ListDocuments(showDeleted)
	if err != nil {
		return nil, nil, nil, err
	}
	names := buildEntityNameMap(store)
	rows, meta, cellRows := documentRows(docs, names)
	return rows, meta, cellRows, nil
}

func (documentHandler) Delete(store *data.Store, id uint) error {
	return store.DeleteDocument(id)
}

func (documentHandler) Restore(store *data.Store, id uint) error {
	return store.RestoreDocument(id)
}

func (documentHandler) StartAddForm(m *Model) error {
	return m.startDocumentForm("")
}

func (documentHandler) StartEditForm(m *Model, id uint) error {
	return m.startEditDocumentForm(id)
}

func (documentHandler) InlineEdit(m *Model, id uint, col int) error {
	return m.inlineEditDocument(id, documentCol(col))
}

func (documentHandler) SubmitForm(m *Model) error {
	return m.submitDocumentForm()
}

func (documentHandler) Snapshot(store *data.Store, id uint) (undoEntry, bool) {
	doc, err := store.GetDocument(id)
	if err != nil {
		return undoEntry{}, false
	}
	return undoEntry{
		Description: fmt.Sprintf("document %q", doc.Title),
		FormKind:    formDocument,
		EntityID:    id,
		Restore: func() error {
			return store.UpdateDocument(doc)
		},
	}, true
}

func (documentHandler) SyncFixedValues(_ *Model, _ []columnSpec) {}

func newEntityDocumentHandler(entityKind string, entityID uint) scopedHandler {
	parent := documentHandler{}
	return scopedHandler{
		TabHandler: parent,
		loadFn: func(store *data.Store, showDeleted bool) ([]table.Row, []rowMeta, [][]cell, error) {
			docs, err := store.ListDocumentsByEntity(entityKind, entityID, showDeleted)
			if err != nil {
				return nil, nil, nil, err
			}
			rows, meta, cellRows := entityDocumentRows(docs)
			return rows, meta, cellRows, nil
		},
		inlineEditFn: skipColEdit(parent, 2), // skip Entity column
		startAddFn: func(m *Model) error {
			return m.startDocumentForm(entityKind)
		},
		submitFn: func(m *Model) error {
			return m.submitScopedDocumentForm(entityKind, entityID)
		},
	}
}
