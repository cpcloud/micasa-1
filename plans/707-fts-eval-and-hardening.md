<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# FTS Eval Harness and Precision Hardening

Follow-up to issue #707.

## Summary

The initial FTS-powered context enrichment (committed in the series starting
with `feat(llm): add FTS-powered context enrichment for chat pipeline`) wired
entity search into all three chat prompt builders but left three known
weaknesses:

1. We have no way to measure whether FTS enrichment actually improves answer
   quality, or on which kinds of questions. Judgment is by eyeball.
2. The FTS query is `LIMIT 20` globally. A single noisy entity type (e.g.
   service_log_entries with long notes) can crowd out all other types from
   the result set, and matches with very weak BM25 scores still get injected
   into the prompt.
3. The index is populated only at app startup. Entities created, renamed, or
   deleted during a session do not surface in FTS until the next launch.
   Entities indexed by derived parent fields (e.g. a quote's `entity_name`
   is `project.title || ' - ' || vendor.name`) hold stale text even after
   restart when the parent renames happened after the child's last write.

This plan addresses all three in one feature series:

- **A.** A `micasa eval fts` subcommand that runs a benchmark question set
  through the live chat pipeline against a fixture DB (or the user's own
  DB), grades each answer with a deterministic rubric plus an LLM judge, and
  reports FTS-on vs FTS-off deltas.
- **B.** `SearchEntities` becomes a single window-function query with a
  per-entity-type quota and a BM25 threshold.
- **C.** `setupEntitiesFTS` installs `AFTER INSERT/UPDATE/DELETE` triggers on
  every source table. The UPDATE trigger on parents whose text is embedded
  in a child's FTS row cascades a refresh to those children. Parent hard
  deletes do not need a cascade block because the foreign-key constraints
  (`RESTRICT` on quote parents, `CASCADE` on SLE parents) make stale child
  FTS rows impossible.

## Current State

`SearchEntities` in `internal/data/fts.go:337` executes:

```sql
SELECT entity_type, entity_id, entity_name, rank
FROM entities_fts
WHERE entities_fts MATCH ?
ORDER BY rank, entity_type, entity_id
LIMIT 20
```

`setupEntitiesFTS` in `internal/data/fts.go:191` drops and rebuilds
`entities_fts` on every `Store.Open`. No triggers exist on source tables.

`buildFTSContext` in `internal/app/chat.go:1165` calls `SearchEntities`, then
`EntitySummary` for each hit, then `llm.BuildFTSContext` to produce the
fenced context block injected into all three prompt builders.

There is no evaluation harness for LLM-driven chat quality anywhere in the
repo. The existing `internal/llm/prompt_test.go` only verifies prompt
assembly; it does not exercise any LLM.

## Design

### A. `micasa eval fts` Subcommand

#### Command Surface

```
micasa eval fts [flags]
    --db PATH             path to a micasa SQLite DB (default: embedded fixture)
    --provider NAME       override chat provider (default: from config)
    --model NAME          override chat model (default: from config)
    --judge-model NAME    model for the judge (default: same as --model)
    --questions NAME,...  run only the named questions (default: all)
    --skip-judge          deterministic rubric only; skip LLM judge
    --no-ab               run each question once (FTS on) instead of twice
    --format markdown|json  (default: markdown)
    --output PATH         write report to file (default: stdout)
    --save-runs DIR       write full prompts/responses per run for debugging
    --strict              exit non-zero if any FTS-on rubric score < FTS-off rubric score
```

`eval` is a new parent subcommand so future evals (extraction, SQL
correctness, prompt-injection resistance) can slot in as siblings.

#### Package Layout

```
cmd/micasa/eval.go                 Cobra wiring for `eval` and `eval fts`
internal/ftseval/                  harness logic (testable without Cobra)
    fixture.go                     typed Go fixture seed for the in-memory DB
    questions.go                   typed Go question set with rubrics
    harness.go                     runner: per-question drive of chat pipeline
    grade.go                       deterministic rubric + judge-call grading
    report.go                      markdown / json output
    ftseval_test.go                coverage for harness/grade/report
```

Keeping the logic in `internal/ftseval/` (rather than inline in
`cmd/micasa`) lets the harness be unit-tested with a mock LLM client, and
keeps Cobra as a thin shell that parses flags and calls the package.

#### Fixture DB

Seeded from typed Go structs, not a checked-in `.db` file. The fixture
covers every indexed entity type and includes deliberate disambiguation
cases:

- Projects: "Kitchen Remodel" (status=in_progress), "Basement Refinish"
  (status=planned), "Roof Replacement" (status=completed).
- Vendors: "Kitchen Supplies Co" (deliberate collision with the project),
  "Pacific Plumbing" (notes: "quoted in February; mentioned permit delays
  for the basement job"), "Acme HVAC".
- Appliances: "Maytag Dishwasher" in kitchen, "Rheem Water Heater" in
  basement, "Carrier Furnace" in basement.
- MaintenanceItems: "HVAC Filter Change" (season=fall), "Gutter Cleaning"
  (season=fall), "Water Heater Flush" (season=spring).
- Incidents: one leak incident referencing the basement location.
- ServiceLogEntries: one entry on "HVAC Filter Change".
- Quotes: one quote linking "Kitchen Remodel" to "Pacific Plumbing".

The fixture is built into an in-memory SQLite database via
`data.OpenInMemory` (new helper if one doesn't exist; otherwise
`data.Open(":memory:")`).

#### Question Set

Each question is a typed Go value:

```go
type Question struct {
    Name              string
    Query             string
    RubricSQL         []*regexp.Regexp  // must all match generated SQL
    RubricSummary     []*regexp.Regexp  // must all match summary text
    ExpectedEntityIDs []string          // entity IDs FTS should surface
    JudgePrompt       string            // graded on 0-5 scale by judge
}
```

Initial set (expandable; typed literals so the compiler protects refactors):

| Name | Query | Tests |
|------|-------|-------|
| kitchen-status | "what's the status of the kitchen project?" | disambiguation vs "Kitchen Supplies" vendor |
| plumber-quote | "how much was the plumber's quote for the kitchen?" | cross-entity join (project + vendor + quote) |
| hvac-last-service | "when was the hvac filter last changed?" | service log lookup via maintenance item |
| total-project-spend | "what's the total I've spent on projects?" | aggregate; FTS should be neutral |
| basement-incidents | "any issues in the basement?" | location-based match across incidents + appliances |
| nonexistent-project | "status of the attic project?" | must not hallucinate; FTS surfaces no project |
| long-tail-note | "the vendor that mentioned permit delays" | find entity only mentioned in notes |
| appliance-by-brand | "list my maytag appliances" | brand field match in appliances |

#### Per-Question Run

For each question:

1. Build both prompts (FTS on and FTS off) via the real builders.
2. Stream SQL generation from the configured provider.
3. Execute the generated SQL via `store.ReadOnlyQuery` against the fixture
   DB. Capture columns, rows, and any error.
4. Stream the summary generation stage with the SQL results.
5. Run the deterministic rubric against `(generatedSQL, summaryText)`.
6. If `--skip-judge` is not set and the preceding stages produced a
   non-empty summary, run one judge call with the user's question, the
   generated SQL, the generated summary, and the `JudgePrompt` criteria.
   The judge returns a 0-5 score and a one-line rationale. If any earlier
   stage failed or the summary is empty, skip the judge entirely and
   record `JudgeScore == -1`.
7. Repeat steps 1-6 with FTS off (unless `--no-ab`).
8. Record per-question result and push into report accumulator.

Runs are sequential by default to avoid rate-limit issues. A future
`--parallel N` flag can be added if needed; not in scope for v1.

#### Grading

`GradeResult` struct:

```go
type GradeResult struct {
    Rubric        int     // passed rubric checks
    RubricTotal   int     // total rubric checks
    JudgeScore    int     // 0-5 when the judge ran; -1 when it did not
    JudgeReason   string  // one-line rationale from the judge; empty when not run
    EntitiesHit   int     // how many ExpectedEntityIDs appeared in FTS context
    EntitiesTotal int
}
```

`JudgeScore == -1` is the sentinel for "judge not run." That covers four
cases: `--skip-judge` was set, an earlier pipeline stage failed so there is
no summary to grade, the judge call itself errored, or the summary was
empty. A genuine `JudgeScore == 0` means the judge ran and found all five
criteria failed — a real quality signal, distinct from "we didn't even
try."

Rubric is rigid regex matching. Judge is a single call with a fixed
meta-prompt that asks the model to score on criteria `C1..C5`:

- C1: Does the answer directly address the question?
- C2: Is the answer grounded in the SQL result (no hallucinated facts)?
- C3: Are entity names correct and disambiguated?
- C4: Is the SQL a reasonable query for the question?
- C5: Is the answer free of irrelevant content?

Each `Ci` is 0 or 1; sum is the 0-5 score.

#### Report

Markdown by default. One row per question, columns:

```
| Question | FTS off rubric | FTS off judge | FTS on rubric | FTS on judge | Δ judge |
```

Plus aggregate footer: total rubric pass rate FTS on vs off, mean judge
score on vs off (computed over rows where `JudgeScore >= 0` only;
`JudgeScore == -1` rows are excluded from the mean and reported as a
separate "judge not run" count), mean tokens used on vs off, mean latency
on vs off. Per-question report cells show `-` when `JudgeScore == -1`.

`--strict` exits non-zero if any per-question FTS-on rubric score is
strictly less than its FTS-off rubric score, over questions that
completed on both arms. Judge-score deltas are reported but never gate
the exit code. See Partial-Failure Handling for the full definition of
"completed" and how incomplete questions are handled.

#### Nix

Add `fts-eval` app in `flake.nix` wrapping the subcommand:

```nix
apps.fts-eval = {
    type = "app";
    program = "${self.packages.${system}.micasa}/bin/micasa";
    # runs: micasa eval fts
};
```

Usage: `nix run '.#fts-eval'`.

### B. Per-Type Quotas and Rank Threshold

Replace the single `LIMIT 20` with a window-function query that enforces
both a per-entity-type cap and a BM25 threshold.

```sql
WITH ranked AS (
    SELECT entity_type, entity_id, entity_name, rank,
           ROW_NUMBER() OVER (PARTITION BY entity_type ORDER BY rank) AS rn
    FROM entities_fts
    WHERE entities_fts MATCH ?
)
SELECT entity_type, entity_id, entity_name, rank
FROM ranked
WHERE rn <= ? AND rank < ?
ORDER BY rank, entity_type, entity_id
LIMIT ?
```

Parameters become package-level consts in `internal/data/fts.go`:

```go
const (
    ftsEntityKPerType    = 5      // max results per entity_type
    ftsEntityRankCeiling = -0.5   // keep only rows with rank strictly less
    ftsEntityTotalCap    = 20     // final cap across all types
)
```

These are not user-configurable (per the repo's "resist configuration"
rule). The eval is the channel for tuning them.

Note on rank semantics: FTS5's BM25 implementation returns **negative**
scores; more relevant results are more negative. `rank < -0.5` keeps only
matches stronger than the -0.5 floor. Values like `-8.0` are very strong;
`-0.1` is a weak coincidental match. The exact floor will likely move after
the eval lands.

SQLite's CTE + window-function combo requires SQLite 3.25 (Sept 2018).
`modernc.org/sqlite` is well past that; no version gate needed.

### C. Triggers with Cascading Refresh

#### Trigger Set

For every source table `<T>` (projects, vendors, appliances,
maintenance_items, incidents, service_log_entries, quotes), install three
triggers:

```sql
-- AFTER INSERT: add a new FTS row matching the concat expression used
-- by populateEntitiesFTS.
CREATE TRIGGER <T>_fts_ai AFTER INSERT ON <T>
WHEN NEW.deleted_at IS NULL
BEGIN
    INSERT INTO entities_fts (entity_type, entity_id, entity_name, entity_text)
    SELECT '<type>', NEW.id, <name-expr>, <text-expr>;
END;

-- AFTER UPDATE: delete any existing FTS row; insert a new one if the
-- row is still visible. Soft-delete (deleted_at -> non-NULL) stops after
-- the delete. Undelete (deleted_at -> NULL) re-inserts.
CREATE TRIGGER <T>_fts_au AFTER UPDATE ON <T>
BEGIN
    DELETE FROM entities_fts
    WHERE entity_type = '<type>' AND entity_id = OLD.id;
    INSERT INTO entities_fts (entity_type, entity_id, entity_name, entity_text)
    SELECT '<type>', NEW.id, <name-expr>, <text-expr>
    WHERE NEW.deleted_at IS NULL;
END;

-- AFTER DELETE: remove the FTS row.
CREATE TRIGGER <T>_fts_ad AFTER DELETE ON <T>
BEGIN
    DELETE FROM entities_fts
    WHERE entity_type = '<type>' AND entity_id = OLD.id;
END;
```

The name and text expressions per table match those already in
`populateEntitiesFTS`. DRY: introduce a Go helper that returns
`(nameExpr, textExpr)` per table and use it in both `populateEntitiesFTS`
and trigger installation.

#### Cascade Rules

Three parent-child relationships where the child's FTS text embeds a parent
field:

| Parent | Child | Parent field in child's FTS | FK on child |
|--------|-------|------------------------------|-------------|
| projects | quotes | project.title feeds quote.entity_name | RESTRICT |
| vendors | quotes | vendor.name feeds quote.entity_name | RESTRICT |
| maintenance_items | service_log_entries | mitem.name feeds SLE.entity_name | CASCADE |

Cascade FTS refresh lives only on the parent's **UPDATE** trigger. Rationale
for not cascading on parent DELETE:

- `quotes.project_id` and `quotes.vendor_id` use `OnDelete:RESTRICT`
  (`internal/data/models.go:179`, `internal/data/models.go:181`), so parent
  hard-deletes with live children are rejected at the DB layer. No stale
  child FTS row can ever exist from this path.
- `service_log_entries.maintenance_item_id` uses `OnDelete:CASCADE`, so
  hard-deleting a maintenance_item hard-deletes its SLEs. Each SLE's own
  `_ad` trigger fires and removes its FTS row. Nothing for the parent
  trigger to do.
- Parent soft-delete is an UPDATE on `deleted_at`. That fires the parent's
  `_au` trigger, which cascades the rebuild to children. Soft-deleted
  parents are filtered from the JOIN (`LEFT JOIN ... AND parent.deleted_at
  IS NULL`), so the rebuilt child row sees NULL for the parent's name and
  the child's `entity_name` degrades accordingly (e.g. the quote becomes
  ` - <vendor>` when the project is soft-deleted). This keeps children's
  FTS text consistent with the parent's own `_au` behavior (soft-deleted
  parent is not searchable as itself).

Example `_au` trigger for `projects`:

```sql
CREATE TRIGGER projects_fts_au AFTER UPDATE ON projects
BEGIN
    -- own row
    DELETE FROM entities_fts
    WHERE entity_type = 'project' AND entity_id = OLD.id;
    INSERT INTO entities_fts (entity_type, entity_id, entity_name, entity_text)
    SELECT 'project', NEW.id,
           NEW.title,
           NEW.title || ' ' || COALESCE(NEW.description, '') || ' ' || COALESCE(NEW.status, '')
    WHERE NEW.deleted_at IS NULL;

    -- cascade: quotes that reference this project
    DELETE FROM entities_fts
    WHERE entity_type = 'quote'
      AND entity_id IN (SELECT id FROM quotes WHERE project_id = OLD.id);
    INSERT INTO entities_fts (entity_type, entity_id, entity_name, entity_text)
    SELECT 'quote', q.id,
           COALESCE(p.title, '') || ' - ' || COALESCE(v.name, ''),
           COALESCE(q.notes, '')
    FROM quotes q
    LEFT JOIN projects p ON q.project_id = p.id AND p.deleted_at IS NULL
    LEFT JOIN vendors v ON q.vendor_id = v.id AND v.deleted_at IS NULL
    WHERE q.project_id = NEW.id AND q.deleted_at IS NULL;
END;
```

Same `_au` shape for `vendors` (cascade into quotes by `vendor_id`) and
`maintenance_items` (cascade into service_log_entries by
`maintenance_item_id`). Parent `_ad` triggers are the plain single-table
form (no cascade block).

Appliances and incidents have no child FTS dependencies; their triggers are
the plain three-trigger form.

#### Installation

`setupEntitiesFTS` extends as follows:

1. Drop and recreate `entities_fts` (existing behavior).
2. `populateEntitiesFTS` — bulk insert from source tables (existing).
3. **New:** for each source table, `DROP TRIGGER IF EXISTS <table>_fts_ai`
   (+ `au`, `ad`), then `CREATE TRIGGER ...`.

Startup continues to be a complete self-heal: dropping the FTS table
removes all its data; recreating it runs populate; reinstalling triggers
ensures schema-drift across app versions is invisible to the runtime.

## Lifecycle Walkthrough

1. **App start** (`Store.Open -> setupFTS -> setupEntitiesFTS`):
   - Drop `entities_fts` and all `<table>_fts_*` triggers.
   - Recreate `entities_fts`.
   - Populate from source tables.
   - Install all triggers: plain three-trigger form for
     appliances/incidents/quotes/service_log_entries; same plus cascade
     block on the `_au` trigger for projects/vendors/maintenance_items.
     `_ad` triggers are plain for every table (no parent `_ad` cascade —
     see Cascade Rules for why).

2. **User adds a project in the TUI** (GORM `Create`):
   - `INSERT INTO projects ...` -> `projects_fts_ai` fires ->
     one row added to `entities_fts`.

3. **User renames a project** (GORM `Save` or `Update`):
   - `UPDATE projects SET title = ... WHERE id = ?` ->
     `projects_fts_au` fires ->
     (1) own FTS row deleted and re-inserted with new title, and
     (2) cascade: every quote referencing that project gets its FTS
     row deleted and re-inserted with the new `title || ' - ' || vendor.name`.

4. **User soft-deletes a project** (GORM default soft-delete):
   - `UPDATE projects SET deleted_at = ? WHERE id = ?` ->
     `projects_fts_au` fires ->
     (1) own FTS row deleted; no re-insert (the `WHERE NEW.deleted_at IS
     NULL` clause suppresses it), and
     (2) cascade into quotes: the LEFT JOIN filters on
     `projects.deleted_at IS NULL`, so `p.title` comes back NULL for
     every affected quote. Quote's `entity_name` becomes ` - <vendor>`;
     the quote stays searchable by vendor name and its own notes, but
     not by the deleted project's title.

5. **Relay sync imports entities from another device** (GORM or raw SQL):
   - Triggers fire because they're DB-level.

6. **Chat query runs**:
   - `buildFTSContext -> SearchEntities` executes the window-function query
     against the already-current index.

7. **App shutdown**:
   - No action. Next startup drops and rebuilds from scratch, so any
     accumulated drift is healed by a relaunch.

## Testing

### Unit Tests (`internal/data/fts_test.go`)

- **Insert surfaces via trigger**: open store, insert a project via GORM,
  call `SearchEntities` with a token from the new title, assert the row
  appears.
- **Update surfaces updated text via trigger**: insert, then rename via
  GORM, search for the new name, assert the FTS row has the new name.
- **Soft-delete removes from index via trigger**: insert, then
  soft-delete, search by any token from the row, assert no result.
- **Hard-delete removes from index via trigger**: insert, then hard-delete,
  search, assert no result.
- **Cascade on parent rename (project)**: insert project + vendor + quote;
  rename the project; search for the new project name; assert the **quote**
  appears (not just the project).
- **Cascade on parent rename (vendor)**: insert project + vendor + quote;
  rename the vendor; search for the new vendor name; assert the quote
  appears.
- **Cascade on parent rename (maintenance_item)**: insert maintenance_item +
  service_log_entry; rename the maintenance item; search for the new name;
  assert the SLE appears.
- **Cascade on parent soft-delete (project)**: insert project + vendor +
  quote; soft-delete the project; search for the quote by vendor name;
  assert the quote still surfaces with degraded `entity_name` (no project
  title).
- **Cascade on parent soft-delete (vendor)**: insert project + vendor +
  quote; soft-delete the vendor; search for the quote by project title;
  assert the quote still surfaces with degraded `entity_name` (no vendor
  name).
- **Cascade on parent soft-delete (maintenance_item)**: insert
  maintenance_item + service_log_entry; soft-delete the maintenance item;
  assert the SLE's FTS row is refreshed (entity_name becomes blank since
  the parent is the only name source) and is still searchable by the SLE
  notes.
- **FK cascade on parent hard-delete**: insert maintenance_item + SLE;
  hard-delete the maintenance_item (FK `OnDelete:CASCADE` removes the SLE);
  assert neither FTS row remains. (Project/vendor hard-delete with live
  quotes is not tested — `OnDelete:RESTRICT` makes it infeasible.)
- **Per-type quota**: insert 10 projects that all match the same token;
  search; assert at most `ftsEntityKPerType` results are type=project.
- **Rank threshold**: insert a row whose text only weakly matches; verify
  it does not appear when a stronger match is available.
- **Total cap**: verify `ftsEntityTotalCap` is enforced even if quotas
  sum higher.

### Chat User-Interaction Tests (`internal/app/chat_test.go`)

Backfills the interaction-test gap in the already-landed FTS integration
commits. Uses the existing `sendKey` / `openAddForm` harness and a mock LLM
client that captures the prompts it receives.

- **FTS context reaches stage 1 prompt**: seed the store with a project
  named "Kitchen Remodel"; submit a chat message "how's the kitchen
  project?" via the real chat input flow (`ctrl+s`); assert the captured
  SQL-generation prompt contains the `--- BEGIN ENTITY DATA ---` fence and
  includes the project id.
- **FTS context reaches stage 2 prompt**: same seed + question; drive the
  pipeline through the SQL execution and into summary generation; assert
  the summary-stage prompt includes the fenced context and the "use solely
  for disambiguation" guardrail sentence.
- **FTS context reaches fallback prompt**: seed store; force stage 1 to
  fail (mock LLM returns empty SQL or an error); assert the fallback
  `BuildSystemPrompt` is called with the FTS context block.
- **No FTS context when store is nil**: sanity check that `buildFTSContext`
  short-circuits cleanly when the store is not attached, and the chat
  pipeline still generates a prompt (without the ENTITY DATA block).

### Harness Tests (`internal/ftseval/ftseval_test.go`)

- Mock LLM client that returns canned SQL + summary.
- Verify harness correctly drives the pipeline, captures both stages,
  applies rubric, and produces the expected report structure.
- Verify `--strict` mode exits non-zero when an FTS-on rubric score is
  below the FTS-off rubric score. Judge scores do not affect `--strict`
  (see Partial-Failure Handling for why); a separate non-strict test
  covers judge-score reporting and delta computation.
- Verify `--no-ab` skips the off-run.
- Verify partial-failure reporting: stage-1 provider error, SQL
  execution error, stage-2 provider error, and judge-call error — each
  recorded with the correct `kind` in the report and `JudgeScore == -1`
  where applicable, without aborting the remaining questions.

### Eval Questions

The question set itself is verified only structurally (each `Name` is
unique, each rubric regex compiles). Semantic correctness is verified by
running the eval, which is the product.

### Coverage Verification

Per the repo's coverage rule, before committing any implementation work for
this plan, run:

```
nix run '.#coverage'
```

Confirm that new and changed lines in `internal/data/fts.go`,
`internal/llm/prompt.go`, `internal/app/chat.go`, `cmd/micasa/eval.go`, and
`internal/ftseval/*.go` are exercised by tests. Coverage gaps in the chat
propagation path are the most likely miss — the user-interaction tests
above are designed to close them.

### Manual Smoke

After wiring, run:

```
nix run '.#fts-eval'
```

against the fixture and check the markdown report prints without errors.
Then run against a real DB and a real provider to confirm end-to-end:

```
micasa eval fts --db ~/.local/share/micasa.db
```

### Acceptance Criteria

The work is considered shippable when **all** of the following hold:

- **Question-set size**: at least 8 questions in the default eval set
  covering disambiguation, cross-entity joins, service-log lookup, an
  aggregate (neutral) case, a nonexistent-entity case, and a long-tail
  notes-only match. Initial table above is the floor.
- **Rubric pass-rate delta**: FTS-on rubric pass rate ≥ FTS-off rubric
  pass rate across the set of questions that completed on both arms
  (same exclusion rule as the aggregate report and `--strict`). No
  completed question may regress in rubric score (FTS-on rubric <
  FTS-off rubric) when run with `--skip-judge`. Incomplete questions
  must be investigated but do not by themselves block the gate.
- **Non-regression guard**: `--strict --skip-judge` against the fixture
  exits zero. CI does not run the judge variant.
- **Judge-variant floor**: when run locally with a judge, the median
  per-question judge-score delta (FTS-on minus FTS-off) must be
  non-negative. The median is computed over the judge-eligible set:
  questions where both arms completed AND both arms returned
  `JudgeScore >= 0`. `judge_error` or parse-failure rows on either arm
  are excluded from this set (they're "complete" for rubric purposes
  but have no judge score to compare). Individual question regressions
  are tolerated because of judge stochasticity, but the median must
  hold.
- **Trigger invariants**: every test listed in the trigger-test section
  above passes. No trigger test may be skipped or marked pending.
- **Coverage**: `nix run '.#coverage'` shows new lines in the target
  files exercised.

## Privacy & Security

The eval subcommand is the one new surface with meaningful privacy
implications. The hardening work itself (triggers, quotas) is entirely
local to the user's SQLite file.

- **`--db` on a real user DB sends household data to the configured
  provider.** The eval uses the user's chat config by default. Every
  question in the set produces a prompt containing FTS-matched entity
  summaries (names, notes, costs, locations). If the provider is a cloud
  service, this data leaves the machine. Print a one-line warning on
  every invocation that is not pointed at the embedded fixture:

  ```
  warning: eval will send prompts derived from <db path> to <provider>.
  Press Ctrl-C within 5s to abort.
  ```

  Suppress the warning only when `--db` is omitted (fixture mode) or when
  the provider is a local one (Ollama, llamacpp, llamafile — already
  tracked in `localProviders`).

- **`--save-runs DIR` writes full prompts and responses to disk.** Same
  sensitivity as above. When set, create files with mode `0600` and write
  a `README.txt` in the directory noting the content is sensitive and
  not committed to git. Do not add `--save-runs` output paths under the
  repo tree; default to refusing to write inside a git working tree
  (resolve the directory's absolute path and refuse if it is inside any
  git checkout).

- **No redaction.** The initial version does not redact any fields. If
  the user needs redaction, they fork the fixture. Document this
  explicitly in `--help` output.

## Partial-Failure Handling

Any LLM call in the per-question pipeline can fail. The harness must
surface failures without aborting the whole run. Failure modes and
handling:

| Failure point | Recorded as | Exit impact |
|---------------|-------------|-------------|
| Stage 1 (NL→SQL) provider error or timeout | `stage1_error` + empty SQL | Rubric fails; judge not run (`JudgeScore == -1`) because there is no summary to grade |
| Generated SQL does not parse / execute | `sql_error` + the DB error | Rubric fails; Stage 2 still runs on the error text (matches production behavior), judge grades whatever Stage 2 produces |
| Stage 2 (summary) provider error | `stage2_error` + empty summary | Rubric fails; judge not run (`JudgeScore == -1`) |
| Judge call error | `judge_error` + `JudgeScore == -1` and `JudgeReason` carries the error text | Treated as "judge not run"; does not affect `--strict` |
| Fixture build failure | hard error, non-zero exit before any question runs | Full abort |

The report's per-question row shows an explicit `error: <kind>` column
when any stage errored. A question counts as **complete** on an arm when
that arm produced a Stage-2 summary — i.e. when `JudgeScore >= 0` or
`--skip-judge` was in effect with no earlier pipeline failure. A question
is **incomplete** when either arm failed before producing a summary
(stage-1 or stage-2 error). `sql_error` alone does not mark a question
incomplete because Stage 2 still runs on the error text per the table
above. Judge-level aggregates (mean judge score, Δ judge) are computed
only over rows where both arms completed and both `JudgeScore >= 0`.
Rubric aggregates always include completed rows; incomplete questions are
listed separately and excluded from both kinds of deltas.

`--strict` only fails on per-question rubric-score regressions between
FTS arms, and only when both arms completed (same exclusion rule as the
aggregate deltas). Incomplete questions — where either arm produced no
Stage-2 summary — are reported but not used to decide the exit code, so a
transient provider error on one arm cannot flip the CI verdict. Judge
errors and judge scores do not affect `--strict` either (judge noise
should not break CI). If the user wants strict judge parity, they run
with both `--strict` and a stable judge model they trust.

## Migration / Rollout

- No schema migration beyond the FTS table (which is rebuilt every
  startup). No data migration for existing users.
- Triggers are installed idempotently via `DROP TRIGGER IF EXISTS`
  followed by `CREATE TRIGGER`, so upgrades overwrite any old version.
- No config keys added; no deprecation work required.
- Backup format unchanged; `entities_fts` is rebuilt from source tables
  on every open, so backups don't need to include FTS state.

### Trigger Drift Detection

Triggers are the only piece of this work that could silently stop
functioning without an obvious symptom (a missing trigger just means the
index grows stale until restart, which then heals it).

- **Startup self-heal** is the primary mechanism: `setupEntitiesFTS`
  drops and reinstalls every trigger on every `Store.Open`. A restart
  therefore restores the invariant unconditionally.
- **Verification helper.** Add `(s *Store) VerifyFTSTriggers() error`
  that queries `sqlite_master` for the expected trigger names and
  returns an error if any are missing. Call it from the existing
  `ping` / health-check paths (surfaced by `micasa status` — see
  #930). No new CLI surface needed.
- **Not in scope for v1:** periodic consistency scans (comparing FTS
  row counts per entity type against source tables). The self-heal
  plus verification helper is sufficient for home-scale usage.

## Risks & Open Questions

- **Trigger performance on bulk import.** The relay sync path can import
  many entities at once. Each row fires one trigger, which is one or two
  FTS row operations. At home scale (hundreds of entities) this is fine.
  If sync-of-initial-household ever grows to thousands of entities, this
  becomes visible. Not a v1 concern; measure if it bites.
- **Cascade trigger fan-out.** Renaming a project with 50 quotes triggers
  50 quote FTS row rebuilds. Acceptable at home scale; worth measuring
  once the eval can run against a real DB with realistic counts.
- **Judge model stability.** LLM judges are stochastic. `--skip-judge`
  is provided as a deterministic fallback; the rubric score alone is
  reproducible. The judge adds signal but should not be the sole gate in
  CI (that's what `--skip-judge` + rubric-only mode is for).
- **Rank threshold tuning.** `-0.5` is a guess. The eval is the tuning
  tool; expect at least one iteration of "run eval, adjust threshold".
- **Eval drift.** The question set is fixture-coupled. If the fixture
  changes, the rubric regexes may drift. Fixture + questions live in the
  same package; changing one without the other fails the harness tests.

## Non-Goals

- No query-aware gating (skip FTS on aggregate-sounding questions). The
  eval will tell us if it's needed later.
- No per-query total-context-size cap. Current per-field truncation via
  `truncateField` remains the only size control.
- No extraction-pipeline FTS integration. That's a separate, larger
  question: extraction doesn't call `BuildSQLPrompt` / `BuildSummaryPrompt`
  and its prompts are structured differently. Tracked separately if
  warranted.
- No configuration knobs for the three FTS tuning constants. They live
  as package-level consts; the eval is the tuning channel.
