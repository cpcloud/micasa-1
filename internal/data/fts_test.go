// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchDocumentsBasic(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Create documents with extracted text.
	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Plumber Receipt",
		FileName:      "receipt.pdf",
		ExtractedText: "Invoice from ABC Plumbing for kitchen sink repair",
		Notes:         "paid in full",
	}))
	require.NoError(t, store.CreateDocument(&Document{
		Title:         "HVAC Manual",
		FileName:      "manual.pdf",
		ExtractedText: "Installation guide for central air conditioning unit",
	}))

	results, err := store.SearchDocuments("plumb")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Plumber Receipt", results[0].Title)
	assert.Contains(t, results[0].Snippet, "Plumb")
}

func TestSearchDocumentsMatchesTitle(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:    "Kitchen Renovation Quote",
		FileName: "quote.pdf",
	}))

	results, err := store.SearchDocuments("kitchen")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Kitchen Renovation Quote", results[0].Title)
}

func TestSearchDocumentsMatchesNotes(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:    "Receipt",
		FileName: "r.pdf",
		Notes:    "emergency plumbing repair on Sunday",
	}))

	results, err := store.SearchDocuments("emergency")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Receipt", results[0].Title)
}

func TestSearchDocumentsExcludesSoftDeleted(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Deleted Doc",
		FileName:      "deleted.pdf",
		ExtractedText: "plumber invoice",
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)

	require.NoError(t, store.DeleteDocument(docs[0].ID))

	results, err := store.SearchDocuments("plumber")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchDocumentsEmptyQuery(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:    "Something",
		FileName: "s.pdf",
	}))

	results, err := store.SearchDocuments("")
	require.NoError(t, err)
	assert.Nil(t, results)

	results, err = store.SearchDocuments("   ")
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestSearchDocumentsMultipleMatches(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Receipt 1",
		FileName:      "r1.pdf",
		ExtractedText: "plumber fixed the kitchen sink",
	}))
	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Receipt 2",
		FileName:      "r2.pdf",
		ExtractedText: "plumber replaced bathroom faucet",
	}))
	require.NoError(t, store.CreateDocument(&Document{
		Title:    "Unrelated",
		FileName: "u.pdf",
	}))

	results, err := store.SearchDocuments("plumber")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestSearchDocumentsPorterStemming(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Painting Invoice",
		FileName:      "inv.pdf",
		ExtractedText: "Professional painting services rendered",
	}))

	// "painted" should match "painting" via porter stemmer (both stem to "paint").
	results, err := store.SearchDocuments("painted")
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestSearchDocumentsUpdateReflected(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Old Title",
		FileName:      "doc.pdf",
		ExtractedText: "original text about gardening",
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	id := docs[0].ID

	// Update extraction text.
	require.NoError(t, store.UpdateDocumentExtraction(id, "new text about plumbing", nil, "", nil))

	results, err := store.SearchDocuments("plumbing")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, id, results[0].ID)

	// Old text should no longer match.
	results, err = store.SearchDocuments("gardening")
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestSearchDocumentsMalformedTokenizes pins the design intent of the
// phrase-wrap escape: when a user types something with stray delimiters
// like "(kitchen" or "kitchen)", the FTS5 tokenizer extracts the inner
// word and the prefix match still works. This is desirable for type-as-
// you-go search where partial input should still surface relevant
// results, not an accidental matching bug.
func TestSearchDocumentsMalformedTokenizes(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Kitchen Renovation",
		FileName:      "k.pdf",
		ExtractedText: "plumber notes",
	}))

	for _, q := range []string{`(kitchen`, `kitchen)`, `"kitchen`, `kitchen*`} {
		t.Run(q, func(t *testing.T) {
			results, err := store.SearchDocuments(q)
			require.NoError(t, err)
			require.Len(t, results, 1, "delimiters around %q should not block tokenization", q)
			assert.Equal(t, "Kitchen Renovation", results[0].Title)
		})
	}
}

// TestSearchDocumentsBadSyntaxGraceful verifies that inputs which would
// be malformed FTS5 expressions if passed verbatim do not error out and
// also do not accidentally match real documents. A document is inserted
// so the no-match assertion is meaningful (an empty store would pass
// even if the query rewrite broadened matches).
func TestSearchDocumentsBadSyntaxGraceful(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Document content uses tokens that don't share any prefix with
	// the test queries below, so any spurious match indicates a bug
	// in the rewrite (e.g., a query collapsing to a bare wildcard).
	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Zebra",
		FileName:      "z.pdf",
		ExtractedText: "rhinoceros giraffe leopard",
	}))

	bad := []string{
		`"unclosed`,
		`unclosed"`,
		`(kitchen`,
		`kitchen)`,
		`((nested`,
		`"phrase with "" inside`,
		`***`,
		`:::`,
		`+++---`,
		`(b AND)`,
	}
	for _, q := range bad {
		t.Run(q, func(t *testing.T) {
			results, err := store.SearchDocuments(q)
			require.NoError(t, err)
			assert.Empty(t, results)
		})
	}
}

func TestSearchDocumentsEntityFields(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Project Doc",
		FileName:      "pd.pdf",
		EntityKind:    DocumentEntityProject,
		EntityID:      "01JTEST00000000000000042",
		ExtractedText: "kitchen renovation details",
	}))

	results, err := store.SearchDocuments("kitchen")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, DocumentEntityProject, results[0].EntityKind)
	assert.Equal(t, "01JTEST00000000000000042", results[0].EntityID)
}

func TestSearchDocumentsSnippetFromBestColumn(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Match is in title only -- snippet should reflect the title.
	require.NoError(t, store.CreateDocument(&Document{
		Title:    "Plumber Receipt",
		FileName: "receipt.pdf",
	}))

	results, err := store.SearchDocuments("plumber")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(
		t,
		results[0].Snippet,
		"Plumb",
		"snippet should come from title when that's the matching column",
	)
}

func TestSearchDocumentsCaseInsensitive(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "HVAC Manual",
		FileName:      "hvac.pdf",
		ExtractedText: "Central Air Conditioning INSTALLATION Guide",
	}))

	// All case variants should match.
	for _, q := range []string{"hvac", "HVAC", "Hvac", "installation", "GUIDE"} {
		results, err := store.SearchDocuments(q)
		require.NoError(t, err, "query %q should not error", q)
		assert.Len(t, results, 1, "query %q should match", q)
	}
}

func TestPrepareFTSQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"hello", `"hello"*`},
		{"hello world", `"hello"* "world"*`},
		// Operators become literal phrase tokens, not FTS5 operators.
		{"a AND b", `"a"* "AND"* "b"*`},
		{`"exact phrase"`, `"""exact"* "phrase"""*`},
		// Internal " is doubled per FTS5's escape rule.
		{`say "hi"`, `"say"* """hi"""*`},
		// All-special tokens stay wrapped; FTS5 tokenizes them to nothing
		// and the phrase matches no documents (verified in integration tests).
		{"***", `"***"*`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, prepareFTSQuery(tt.in))
		})
	}
}

func TestPrepareFTSEntityQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"all stopwords", "what is the of", ""},
		{"single content word", "plumber", `"plumber"*`},
		{
			"strips stopwords and punctuation",
			"what's the status of the kitchen project?",
			`"status"* OR "kitchen"* OR "project"*`,
		},
		{
			"or-joins multiple content words",
			"kitchen remodel budget",
			`"kitchen"* OR "remodel"* OR "budget"*`,
		},
		{
			"drops 1-char tokens",
			"a b c kitchen",
			`"kitchen"*`,
		},
		{
			"drops pure punctuation",
			"- ? kitchen !",
			`"kitchen"*`,
		},
		{
			"lowercases",
			"Kitchen REMODEL",
			`"kitchen"* OR "remodel"*`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, prepareFTSEntityQuery(tt.in))
		})
	}
}

// TestSearchEntitiesMatchesNaturalLanguageQuestions is the end-to-end
// regression for the stopword-AND bug: before the prepareFTSEntityQuery
// fix, asking a conversational question like "what's the status of the
// kitchen project?" produced zero results because every word had to
// match. Now the content words OR-match and the kitchen project
// surfaces.
func TestSearchEntitiesMatchesNaturalLanguageQuestions(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title:         "Kitchen Remodel",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusInProgress,
	}))

	for _, q := range []string{
		"what's the status of the kitchen project?",
		"how's the kitchen going",
		"kitchen",
	} {
		t.Run(q, func(t *testing.T) {
			results, err := store.SearchEntities(q)
			require.NoError(t, err)
			require.NotEmpty(t, results, "expected a match for %q", q)
			assert.Equal(t, "Kitchen Remodel", results[0].EntityName)
		})
	}
}

// TestSearchEntitiesStopwordOnlyQueryReturnsEmpty covers the fast
// path where every user token is a stopword. The expected behavior
// is "no results" rather than "match everything".
func TestSearchEntitiesStopwordOnlyQueryReturnsEmpty(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title:         "Kitchen Remodel",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusInProgress,
	}))

	results, err := store.SearchEntities("what is the")
	require.NoError(t, err)
	assert.Empty(t, results, "stopword-only query must not match every row")
}

func TestRebuildFTSIndex(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Test Doc",
		FileName:      "t.pdf",
		ExtractedText: "searchable content here",
	}))

	require.NoError(t, store.RebuildFTSIndex())

	results, err := store.SearchDocuments("searchable")
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestHasFTSTable(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	assert.True(t, store.hasFTSTable())
}

// --- entities_fts tests ---

func TestSetupEntitiesFTSCreatesTable(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	var count int64
	store.db.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		"entities_fts",
	).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestSetupEntitiesFTSPopulatesProjects(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title:         "Kitchen Remodel",
		Description:   "Full kitchen renovation",
		Status:        ProjectStatusInProgress,
		ProjectTypeID: types[0].ID,
	}))

	require.NoError(t, store.setupEntitiesFTS())

	var count int64
	store.db.Raw(`SELECT COUNT(*) FROM entities_fts WHERE entity_type = 'project'`).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestSetupEntitiesFTSPopulatesVendors(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Test Plumber", ContactName: "John"}))
	require.NoError(t, store.setupEntitiesFTS())

	var count int64
	store.db.Raw(`SELECT COUNT(*) FROM entities_fts WHERE entity_type = 'vendor'`).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestSetupEntitiesFTSPopulatesAppliances(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateAppliance(&Appliance{Name: "HVAC Unit", Brand: "Carrier"}))
	require.NoError(t, store.setupEntitiesFTS())

	var count int64
	store.db.Raw(`SELECT COUNT(*) FROM entities_fts WHERE entity_type = 'appliance'`).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestSetupEntitiesFTSPopulatesIncidents(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateIncident(&Incident{
		Title:    "Roof Leak",
		Status:   IncidentStatusOpen,
		Severity: IncidentSeverityUrgent,
	}))
	require.NoError(t, store.setupEntitiesFTS())

	var count int64
	store.db.Raw(`SELECT COUNT(*) FROM entities_fts WHERE entity_type = 'incident'`).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestSetupEntitiesFTSExcludesSoftDeleted(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Active Vendor"}))
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Deleted Vendor"}))

	vendors, _ := store.ListVendors(false)
	require.Len(t, vendors, 2)
	require.NoError(t, store.DeleteVendor(vendors[1].ID))

	require.NoError(t, store.setupEntitiesFTS())

	var count int64
	store.db.Raw(`SELECT COUNT(*) FROM entities_fts WHERE entity_type = 'vendor'`).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestSearchEntitiesBasic(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Kitchen Remodel", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	require.NoError(t, store.CreateVendor(&Vendor{Name: "ABC Plumbing"}))
	require.NoError(t, store.setupEntitiesFTS())

	results, err := store.SearchEntities("kitchen")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "project", results[0].EntityType)
	assert.Equal(t, "Kitchen Remodel", results[0].EntityName)
}

func TestSearchEntitiesEmptyQuery(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	results, err := store.SearchEntities("")
	require.NoError(t, err)
	assert.Nil(t, results)

	results, err = store.SearchEntities("   ")
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestSearchEntitiesBadSyntaxGraceful(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	results, err := store.SearchEntities(`"unclosed`)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchEntitiesCrossEntityMatches(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Plumbing Overhaul", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Pro Plumbing"}))
	require.NoError(t, store.setupEntitiesFTS())

	results, err := store.SearchEntities("plumbing")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestSearchEntitiesNoFTSTableGraceful(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.db.Exec("DROP TABLE IF EXISTS entities_fts").Error)

	results, err := store.SearchEntities("anything")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchEntitiesWrapsQueryError(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Replace the well-formed entities_fts with a virtual table that has a
	// different column set, so hasEntitiesFTSTable still reports true but
	// the SELECT against the expected columns errors at query time.
	require.NoError(t, store.db.Exec("DROP TABLE IF EXISTS entities_fts").Error)
	require.NoError(t, store.db.Exec(
		"CREATE VIRTUAL TABLE entities_fts USING fts5(unrelated_column)",
	).Error)

	results, err := store.SearchEntities("anything")
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "search entities:")
}

func TestSearchEntitiesPorterStemming(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Professional Painting Services"}))
	require.NoError(t, store.setupEntitiesFTS())

	// "painted" should match "painting" via porter stemmer (both stem to "paint").
	results, err := store.SearchEntities("painted")
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestEntitySummaryProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	budget := int64(1500000)
	require.NoError(t, store.CreateProject(&Project{
		Title: "Kitchen Remodel", ProjectTypeID: types[0].ID,
		Status: ProjectStatusInProgress, BudgetCents: &budget,
	}))
	projects, _ := store.ListProjects(false)
	require.Len(t, projects, 1)

	summary, found, err := store.EntitySummary("project", projects[0].ID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Contains(t, summary, "Kitchen Remodel")
	assert.Contains(t, summary, "underway")
	assert.Contains(t, summary, "$15000.00")
}

func TestEntitySummaryVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{
		Name: "ABC Plumbing", ContactName: "John Smith", Phone: "555-0123",
	}))
	vendors, _ := store.ListVendors(false)

	summary, found, err := store.EntitySummary("vendor", vendors[0].ID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Contains(t, summary, "ABC Plumbing")
	assert.Contains(t, summary, "contact=John Smith")
	assert.Contains(t, summary, "phone=555-0123")
}

func TestEntitySummaryAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateAppliance(&Appliance{
		Name: "Dishwasher", Brand: "LG", ModelNumber: "WM3900",
	}))
	appliances, _ := store.ListAppliances(false)

	summary, found, err := store.EntitySummary("appliance", appliances[0].ID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Contains(t, summary, "Dishwasher")
	assert.Contains(t, summary, "brand=LG")
	assert.Contains(t, summary, "model=WM3900")
}

func TestEntitySummaryDeletedEntity(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Gone Vendor"}))
	vendors, _ := store.ListVendors(false)
	require.NoError(t, store.DeleteVendor(vendors[0].ID))

	_, found, err := store.EntitySummary("vendor", vendors[0].ID)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestEntitySummaryUnknownType(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	_, found, err := store.EntitySummary("nonexistent", "01JFAKE")
	require.Error(t, err)
	assert.False(t, found)
}

func TestEntitySummaryRevalidatesStaleIndex(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Will Be Deleted"}))
	require.NoError(t, store.setupEntitiesFTS())

	results, err := store.SearchEntities("deleted")
	require.NoError(t, err)
	require.Len(t, results, 1)

	vendors, _ := store.ListVendors(false)
	require.NoError(t, store.DeleteVendor(vendors[0].ID))

	_, found, err := store.EntitySummary(results[0].EntityType, results[0].EntityID)
	require.NoError(t, err)
	assert.False(t, found, "deleted entity should not be found via EntitySummary")
}

func TestEntitySummaryIncident(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Roof Leak", Status: IncidentStatusOpen,
		Severity: IncidentSeverityUrgent, Location: "attic",
	}))
	incidents, _ := store.ListIncidents(false)

	summary, found, err := store.EntitySummary("incident", incidents[0].ID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Contains(t, summary, "Roof Leak")
	assert.Contains(t, summary, "status=open")
	assert.Contains(t, summary, "severity=urgent")
	assert.Contains(t, summary, "location=attic")
}

func TestTruncateField(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "short", truncateField("short"))
	long := strings.Repeat("a", 300)
	result := truncateField(long)
	assert.Len(t, result, 203) // 200 + "..."
	assert.True(t, strings.HasSuffix(result, "..."))
}

func TestTruncateFieldUnicode(t *testing.T) {
	t.Parallel()
	// 201 runes of multi-byte characters should truncate at rune boundary.
	s := strings.Repeat("\u00e9", 201) // e-acute
	result := truncateField(s)
	runes := []rune(result)
	// 200 runes + "..." = 203 runes
	assert.Len(t, runes, 203)
}

func TestHasEntitiesFTSTable(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	assert.True(t, store.hasEntitiesFTSTable())
}

// ---------------------------------------------------------------------------
// Trigger tests: verify that AI / AU / AD triggers keep entities_fts in sync
// with source-table writes without a manual setupEntitiesFTS rebuild.
// ---------------------------------------------------------------------------

func TestFTSTriggerInsertSurfacesProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title:         "Greenhouse Build",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}))

	results, err := store.SearchEntities("greenhouse")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, DeletionEntityProject, results[0].EntityType)
	assert.Equal(t, "Greenhouse Build", results[0].EntityName)
}

func TestFTSTriggerUpdateSurfacesNewTitle(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	p := &Project{
		Title:         "Old Title",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))

	p.Title = "Fresh Greenhouse"
	require.NoError(t, store.UpdateProject(*p))

	// Old token no longer surfaces.
	oldResults, err := store.SearchEntities("old")
	require.NoError(t, err)
	assert.Empty(t, oldResults, "old title should be gone from FTS")

	// New token surfaces.
	newResults, err := store.SearchEntities("greenhouse")
	require.NoError(t, err)
	require.Len(t, newResults, 1)
	assert.Equal(t, "Fresh Greenhouse", newResults[0].EntityName)
}

func TestFTSTriggerSoftDeleteRemovesRow(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	p := &Project{
		Title:         "Transient Project",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))

	// Sanity: it's indexed.
	before, err := store.SearchEntities("transient")
	require.NoError(t, err)
	require.Len(t, before, 1)

	require.NoError(t, store.DeleteProject(p.ID))

	after, err := store.SearchEntities("transient")
	require.NoError(t, err)
	assert.Empty(t, after, "soft-deleted project must not surface")
}

func TestFTSTriggerCascadeOnProjectRename(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	p := &Project{
		Title:         "Kitchen Remodel",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))

	v := &Vendor{Name: "Pacific Plumbing"}
	require.NoError(t, store.CreateVendor(v))

	require.NoError(t, store.CreateQuote(&Quote{
		ProjectID:  p.ID,
		VendorID:   v.ID,
		TotalCents: 1000,
	}, *v))

	// Rename the project.
	p.Title = "Greenhouse Build"
	require.NoError(t, store.UpdateProject(*p))

	// The quote should now be findable by the new project name.
	results, err := store.SearchEntities("greenhouse")
	require.NoError(t, err)

	var quoteFound bool
	for _, r := range results {
		if r.EntityType == DeletionEntityQuote {
			quoteFound = true
			break
		}
	}
	assert.True(
		t,
		quoteFound,
		"cascade should rebuild quote FTS with new project title; got %+v",
		results,
	)
}

func TestFTSTriggerCascadeOnVendorRename(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	p := &Project{
		Title:         "Basement Refinish",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))

	v := &Vendor{Name: "Old Vendor Name"}
	require.NoError(t, store.CreateVendor(v))

	require.NoError(t, store.CreateQuote(&Quote{
		ProjectID:  p.ID,
		VendorID:   v.ID,
		TotalCents: 2000,
	}, *v))

	v.Name = "Aurora Plumbing"
	require.NoError(t, store.UpdateVendor(*v))

	results, err := store.SearchEntities("aurora")
	require.NoError(t, err)

	var quoteFound bool
	for _, r := range results {
		if r.EntityType == DeletionEntityQuote {
			quoteFound = true
			break
		}
	}
	assert.True(
		t,
		quoteFound,
		"cascade should rebuild quote FTS with new vendor name; got %+v",
		results,
	)
}

func TestFTSTriggerCascadeOnMaintenanceRename(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	m := &MaintenanceItem{
		Name:           "Old Name",
		CategoryID:     cats[0].ID,
		IntervalMonths: 6,
	}
	require.NoError(t, store.CreateMaintenance(m))

	sle := &ServiceLogEntry{
		MaintenanceItemID: m.ID,
		ServicedAt:        time.Now(),
	}
	require.NoError(t, store.CreateServiceLog(sle, Vendor{}))

	m.Name = "Quarterly Furnace Check"
	require.NoError(t, store.UpdateMaintenance(*m))

	results, err := store.SearchEntities("furnace")
	require.NoError(t, err)

	var sleFound bool
	for _, r := range results {
		if r.EntityType == DeletionEntityServiceLog {
			sleFound = true
			break
		}
	}
	assert.True(
		t,
		sleFound,
		"cascade should rebuild SLE FTS with new maintenance item name; got %+v",
		results,
	)
}

func TestFTSTriggerCascadeOnProjectSoftDelete(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	p := &Project{
		Title:         "Attic Insulation",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))

	v := &Vendor{Name: "Summit Insulators"}
	require.NoError(t, store.CreateVendor(v))

	require.NoError(t, store.CreateQuote(&Quote{
		ProjectID:  p.ID,
		VendorID:   v.ID,
		TotalCents: 3000,
	}, *v))

	// App-level DeleteProject refuses soft-delete when a project has live
	// quotes. The trigger's cascade path is still reachable via sync and
	// future app changes, so exercise it via raw DML that bypasses the
	// validation — the goal is to prove the DB trigger behaves correctly
	// when the scenario arises, not to test DeleteProject's gating.
	require.NoError(t, store.db.Exec(
		"UPDATE "+TableProjects+" SET "+ColDeletedAt+" = ? WHERE "+ColID+" = ?",
		time.Now(), p.ID,
	).Error)

	// Searching by vendor name should still surface the quote (with a
	// degraded entity_name now that the project title is gone).
	results, err := store.SearchEntities("summit")
	require.NoError(t, err)

	var quoteFound bool
	for _, r := range results {
		if r.EntityType == DeletionEntityQuote {
			quoteFound = true
			assert.NotContains(t, r.EntityName, "Attic Insulation",
				"soft-deleted project title must not be in child entity_name")
		}
	}
	assert.True(t, quoteFound, "quote should still surface via vendor name; got %+v", results)

	// And searching by the now-gone project title should NOT find the quote.
	attic, err := store.SearchEntities("attic")
	require.NoError(t, err)
	assert.Empty(t, attic, "soft-deleted project title should not surface via any entity")
}

// ---------------------------------------------------------------------------
// Per-type quota and rank threshold tests (ftsEntityKPerType,
// ftsEntityRankCeiling, ftsEntityTotalCap).
// ---------------------------------------------------------------------------

func TestFTSSearchEntitiesPerTypeQuotaGuaranteesRepresentation(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Insert many projects that all share a strong matching token. With
	// no per-type quota, the lone matching vendor below would drop off
	// the bottom as projects dominate the top of the ranking. The quota
	// guarantees at least one vendor slot; remaining space is still
	// filled from the global top.
	types, _ := store.ProjectTypes()
	const projectCount = 10
	for i := range projectCount {
		require.NoError(t, store.CreateProject(&Project{
			Title:         fmt.Sprintf("Sawmill Project %d", i),
			ProjectTypeID: types[0].ID,
			Status:        ProjectStatusPlanned,
		}))
	}

	// Single vendor matching the same token. Without the quota this
	// would be at rank position 11 behind all 10 projects; with the
	// quota tier 1 forces it into the result set.
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Sawmill Supplies Co"}))

	results, err := store.SearchEntities("sawmill")
	require.NoError(t, err)

	var vendorHits int
	for _, r := range results {
		if r.EntityType == DeletionEntityVendor {
			vendorHits++
		}
	}
	assert.Equal(t, 1, vendorHits,
		"vendor must survive the project flood thanks to the per-type quota; got %d", vendorHits)
	assert.LessOrEqual(t, len(results), ftsEntityTotalCap,
		"total cap must still hold")
}

func TestFTSSearchEntitiesTotalCap(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Insert many entities across multiple types so (ftsEntityKPerType *
	// number_of_types) > ftsEntityTotalCap. The overall LIMIT must still
	// apply.
	types, _ := store.ProjectTypes()
	for i := range ftsEntityKPerType + 2 {
		require.NoError(t, store.CreateProject(&Project{
			Title:         fmt.Sprintf("Overflow Project %d", i),
			ProjectTypeID: types[0].ID,
			Status:        ProjectStatusPlanned,
		}))
	}
	for i := range ftsEntityKPerType + 2 {
		require.NoError(t, store.CreateVendor(&Vendor{
			Name: fmt.Sprintf("Overflow Vendor %d", i),
		}))
	}
	for i := range ftsEntityKPerType + 2 {
		require.NoError(t, store.CreateAppliance(&Appliance{
			Name: fmt.Sprintf("Overflow Appliance %d", i),
		}))
	}
	for i := range ftsEntityKPerType + 2 {
		require.NoError(t, store.CreateIncident(&Incident{
			Title:    fmt.Sprintf("Overflow Incident %d", i),
			Status:   "open",
			Severity: "low",
		}))
	}

	results, err := store.SearchEntities("overflow")
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), ftsEntityTotalCap,
		"total cap should limit results to %d; got %d", ftsEntityTotalCap, len(results))
}

func TestFTSSearchEntitiesSingleTypeUsesFullCap(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Insert more projects than the per-type quota, with NO other type
	// matching. The earlier flat-quota implementation would clip at
	// ftsEntityKPerType even though 15 other slots were unused; the
	// two-tier implementation should fill up to ftsEntityTotalCap.
	types, _ := store.ProjectTypes()
	const projectCount = ftsEntityKPerType + 3
	for i := range projectCount {
		require.NoError(t, store.CreateProject(&Project{
			Title:         fmt.Sprintf("Lakeside Project %d", i),
			ProjectTypeID: types[0].ID,
			Status:        ProjectStatusPlanned,
		}))
	}

	results, err := store.SearchEntities("lakeside")
	require.NoError(t, err)
	assert.GreaterOrEqual(
		t,
		len(results),
		projectCount,
		"single-type search must not be capped at ftsEntityKPerType when no other type competes; got %d",
		len(results),
	)
	assert.LessOrEqual(t, len(results), ftsEntityTotalCap,
		"total cap should still apply")
}

func TestFTSSearchEntitiesTiebreakerIsDeterministic(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Insert several projects with identical text so BM25 assigns them
	// all the same rank. Run the search twice and assert results come
	// back in the same order -- the window ORDER BY has an entity_id
	// tiebreaker to guarantee this.
	types, _ := store.ProjectTypes()
	const count = ftsEntityKPerType + 2
	for i := range count {
		require.NoError(t, store.CreateProject(&Project{
			Title:         fmt.Sprintf("Identical Widget %d", i),
			ProjectTypeID: types[0].ID,
			Status:        ProjectStatusPlanned,
		}))
	}

	first, err := store.SearchEntities("widget")
	require.NoError(t, err)
	require.NotEmpty(t, first)
	second, err := store.SearchEntities("widget")
	require.NoError(t, err)
	require.Equal(t, len(first), len(second), "same query should return same count")
	for i := range first {
		assert.Equal(t, first[i].EntityID, second[i].EntityID,
			"position %d should be stable across runs", i)
	}
}

func TestFTSSearchEntitiesRepresentsEveryMatchingType(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Insert enough entities per type that each type's match count
	// would otherwise exceed the per-type quota, so the tier-3 fill
	// has a chance to shadow late types. With the "one row per matching
	// type first" tier, every matching type should still surface.
	types, _ := store.ProjectTypes()
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	const perType = ftsEntityKPerType + 3
	for i := range perType {
		require.NoError(t, store.CreateProject(&Project{
			Title:         fmt.Sprintf("Signal Project %d", i),
			ProjectTypeID: types[0].ID,
			Status:        ProjectStatusPlanned,
		}))
		require.NoError(t, store.CreateVendor(&Vendor{
			Name: fmt.Sprintf("Signal Vendor %d", i),
		}))
		require.NoError(t, store.CreateAppliance(&Appliance{
			Name: fmt.Sprintf("Signal Appliance %d", i),
		}))
		require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
			Name:           fmt.Sprintf("Signal Maintenance %d", i),
			CategoryID:     cats[0].ID,
			IntervalMonths: 6,
		}))
		require.NoError(t, store.CreateIncident(&Incident{
			Title:    fmt.Sprintf("Signal Incident %d", i),
			Status:   "open",
			Severity: "low",
		}))
	}

	results, err := store.SearchEntities("signal")
	require.NoError(t, err)

	seen := map[string]bool{}
	for _, r := range results {
		seen[r.EntityType] = true
	}
	for _, entity := range []string{
		DeletionEntityProject,
		DeletionEntityVendor,
		DeletionEntityAppliance,
		DeletionEntityMaintenance,
		DeletionEntityIncident,
	} {
		assert.True(t, seen[entity],
			"every matching type must appear at least once; %s missing", entity)
	}
}

func TestFTSPopulateFiltersSoftDeletedMaintenanceInSLEJoin(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	m := &MaintenanceItem{
		Name:           "Rebuild Maintenance Name",
		CategoryID:     cats[0].ID,
		IntervalMonths: 12,
	}
	require.NoError(t, store.CreateMaintenance(m))

	sle := &ServiceLogEntry{
		MaintenanceItemID: m.ID,
		ServicedAt:        time.Now(),
		Notes:             "still-alive notes",
	}
	require.NoError(t, store.CreateServiceLog(sle, Vendor{}))

	// App-level DeleteMaintenance validation would reject this with a
	// live SLE, so bypass via raw SQL to simulate the sync / future
	// scenario where the parent arrives soft-deleted.
	require.NoError(t, store.db.Exec(
		"UPDATE "+TableMaintenanceItems+" SET "+ColDeletedAt+" = ? WHERE "+ColID+" = ?",
		time.Now(), m.ID,
	).Error)

	// Force the initial-rebuild path.
	require.NoError(t, store.setupEntitiesFTS())

	results, err := store.SearchEntities("rebuild")
	require.NoError(t, err)
	for _, r := range results {
		if r.EntityType == DeletionEntityServiceLog {
			assert.NotContains(t, r.EntityName, "Rebuild Maintenance Name",
				"initial rebuild must not carry soft-deleted maintenance name into SLE FTS")
		}
	}
}

func TestFTSPopulateFiltersSoftDeletedParentsInQuoteJoin(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Create project + vendor + quote, soft-delete the project via raw
	// SQL (the app-level DeleteProject rejects parents with live quotes),
	// then run the initial rebuild path. The quote's FTS row must not
	// carry the deleted project's title.
	types, _ := store.ProjectTypes()
	p := &Project{
		Title:         "Rebuild Project Title",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))
	v := &Vendor{Name: "Rebuild Vendor Name"}
	require.NoError(t, store.CreateVendor(v))
	require.NoError(t, store.CreateQuote(&Quote{
		ProjectID:  p.ID,
		VendorID:   v.ID,
		TotalCents: 1000,
	}, *v))

	require.NoError(t, store.db.Exec(
		"UPDATE "+TableProjects+" SET "+ColDeletedAt+" = ? WHERE "+ColID+" = ?",
		time.Now(), p.ID,
	).Error)

	// Force the initial-rebuild path (mirrors what happens on app open).
	require.NoError(t, store.setupEntitiesFTS())

	rebuild, err := store.SearchEntities("rebuild")
	require.NoError(t, err)
	for _, r := range rebuild {
		if r.EntityType == DeletionEntityQuote {
			assert.NotContains(t, r.EntityName, "Rebuild Project Title",
				"initial rebuild must not carry soft-deleted project title into quote FTS")
		}
	}
}

func TestFTSSearchEntitiesRankThresholdFiltersAboveCeiling(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Prove the rank threshold infrastructure works: insert a vendor with
	// a known searchable name, then verify that every returned row has
	// `rank < ftsEntityRankCeiling`. The initial ceiling is permissive
	// (0.0) — every BM25 match is negative, so every result passes. Once
	// the eval tightens the ceiling, this test continues to hold.
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Rank Threshold Test Co"}))

	results, err := store.SearchEntities("threshold")
	require.NoError(t, err)
	require.NotEmpty(t, results, "vendor name should match")

	for _, r := range results {
		assert.Less(t, r.Rank, ftsEntityRankCeiling,
			"every returned row must have rank < %v; got %q with rank %v",
			ftsEntityRankCeiling, r.EntityName, r.Rank)
	}
}

func TestFTSTriggerHardDeleteMaintenanceCascadesSLE(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	m := &MaintenanceItem{
		Name:           "Gutter Cleaning",
		CategoryID:     cats[0].ID,
		IntervalMonths: 12,
	}
	require.NoError(t, store.CreateMaintenance(m))

	sle := &ServiceLogEntry{
		MaintenanceItemID: m.ID,
		ServicedAt:        time.Now(),
		Notes:             "fall cleanup",
	}
	require.NoError(t, store.CreateServiceLog(sle, Vendor{}))

	require.NoError(t, store.HardDeleteMaintenance(m.ID))

	gutterResults, err := store.SearchEntities("gutter")
	require.NoError(t, err)
	assert.Empty(t, gutterResults, "maintenance item FTS row should be gone after hard delete")

	fallResults, err := store.SearchEntities("fall")
	require.NoError(t, err)
	assert.Empty(t, fallResults, "child SLE FTS row should be gone via FK cascade + _ad trigger")
}
