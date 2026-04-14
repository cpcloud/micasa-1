<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# `/test-pr` Skill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a Claude Code skill that builds, seeds, and launches
micasa from any PR branch or worktree in a tmux session for interactive
testing.

**Architecture:** Single skill file (`.claude/commands/test-pr.md`)
that instructs Claude to resolve input → worktree, build the binary,
seed a test DB, and launch in tmux. Three modes: single-branch
(interactive), A/B comparison (two panes), and smoke test (automated).

**Tech Stack:** tmux, direnv, go build, gh CLI

---

## File Map

- Create: `.claude/commands/test-pr.md` — the skill file

No Go code, no tests. This is a Claude Code skill (markdown
instructions). Verification is manual invocation.

---

### Task 1: Create skill file with argument parsing and input resolution

**Files:**
- Create: `.claude/commands/test-pr.md`

- [ ] **Step 1: Create the skill file skeleton**

```markdown
---
name: test-pr
description: Build and launch a PR branch in tmux for interactive testing. Supports single-branch, A/B comparison, and smoke test modes.
---

# Test PR

Build a PR branch, seed a test database, and launch in tmux for
interactive testing.

## Arguments

- First argument: PR number, worktree name, or branch name
- `--ab`: A/B comparison mode (optional second argument for second
  branch; defaults to main)
- `--smoke`: automated smoke test mode (non-interactive)

## Input Resolution

Resolve the input to a worktree path:

- **PR number** (all digits): run `gh pr view <number> --repo
  micasa-dev/micasa --json headRefName --jq .headRefName` to get the
  branch name, then find a matching worktree.
- **Worktree name**: check if `.claude/worktrees/<name>` exists.
- **Branch name**: check `git worktree list` for a worktree on that
  branch.

To find a worktree matching a branch name, run:

```sh
git worktree list --porcelain | grep -B2 "branch refs/heads/<branch>" | head -1 | sed 's/worktree //'
```

If no worktree exists, create a temporary one:

```sh
git worktree add .claude/worktrees/test-pr-<branch> <branch>
```

Store the resolved worktree path for use in subsequent steps.
```

- [ ] **Step 2: Commit**

```
git add .claude/commands/test-pr.md
```

Commit with: `docs: add test-pr skill skeleton with input resolution`

---

### Task 2: Add single-branch mode (build, seed, launch, handoff)

**Files:**
- Modify: `.claude/commands/test-pr.md`

- [ ] **Step 1: Add the single-branch section**

Append after the Input Resolution section:

```markdown
## Single-Branch Mode (default)

Used when neither `--ab` nor `--smoke` is passed.

### Build

Build the binary from the resolved worktree:

```sh
direnv exec <worktree> bash -c 'cd <worktree> && go build -o /tmp/micasa-pr-<id> ./cmd/micasa'
```

Where `<id>` is the PR number, worktree name, or branch name
(sanitized for filesystem use). Report build errors and stop if
the build fails.

### Seed database

```sh
/tmp/micasa-pr-<id> demo --seed-only /tmp/micasa-test-<id>.db
```

This creates a fresh SQLite database with demo data. Previous
databases at the same path are overwritten.

### Kill stale session

```sh
tmux -L claude-tui kill-session -t pr-<id> 2>/dev/null
```

### Launch in tmux

```sh
tmux -L claude-tui new-session -d -s pr-<id> -x 120 -y 40
tmux -L claude-tui send-keys -t pr-<id> '/tmp/micasa-pr-<id> /tmp/micasa-test-<id>.db' Enter
```

Wait 2 seconds, then capture the pane to verify the app started:

```sh
sleep 2
tmux -L claude-tui capture-pane -t pr-<id> -p
```

If the capture shows a panic or shell prompt (app exited), report
the error.

### Handoff

Print this to the user:

```
Built from <worktree> (branch <branch>)
DB seeded at /tmp/micasa-test-<id>.db

Attach with:  ! tmux -L claude-tui attach -t pr-<id>

Detach with Ctrl+B, D to return here.
Kill session: ! tmux -L claude-tui kill-session -t pr-<id>
```
```

- [ ] **Step 2: Commit**

Commit with: `docs: add single-branch mode to test-pr skill`

---

### Task 3: Add A/B comparison mode

**Files:**
- Modify: `.claude/commands/test-pr.md`

- [ ] **Step 1: Add A/B section**

Append after Single-Branch Mode:

```markdown
## A/B Comparison Mode (`--ab`)

Builds and launches two branches side by side in a split tmux session.

### Resolve both sides

- **Left pane**: the first argument (resolved as above)
- **Right pane**: the second argument if provided (resolved the same
  way), otherwise main. For main, use the repository's main checkout
  directory (parent of `.claude/worktrees/`).

### Build both binaries

```sh
direnv exec <worktree-left> bash -c 'cd <worktree-left> && go build -o /tmp/micasa-ab-left ./cmd/micasa'
direnv exec <worktree-right> bash -c 'cd <worktree-right> && go build -o /tmp/micasa-ab-right ./cmd/micasa'
```

Build these in parallel (two Bash tool calls). Report errors and
stop if either fails.

### Seed both databases

```sh
/tmp/micasa-ab-left demo --seed-only /tmp/micasa-ab-left.db
/tmp/micasa-ab-right demo --seed-only /tmp/micasa-ab-right.db
```

### Kill stale session and launch

```sh
tmux -L claude-tui kill-session -t ab-<id> 2>/dev/null
tmux -L claude-tui new-session -d -s ab-<id> -x 240 -y 40
tmux -L claude-tui send-keys -t ab-<id> '/tmp/micasa-ab-left /tmp/micasa-ab-left.db' Enter
tmux -L claude-tui split-window -h -t ab-<id>
tmux -L claude-tui send-keys -t ab-<id> '/tmp/micasa-ab-right /tmp/micasa-ab-right.db' Enter
```

Wait 3 seconds, capture both panes to verify startup.

### Handoff

```
A/B comparison: <left-label> (left) vs <right-label> (right)
DBs: /tmp/micasa-ab-left.db, /tmp/micasa-ab-right.db

Attach with:  ! tmux -L claude-tui attach -t ab-<id>

Switch panes: Ctrl+B, Arrow
Detach: Ctrl+B, D
Kill: ! tmux -L claude-tui kill-session -t ab-<id>
```
```

- [ ] **Step 2: Commit**

Commit with: `docs: add A/B comparison mode to test-pr skill`

---

### Task 4: Add smoke test mode

**Files:**
- Modify: `.claude/commands/test-pr.md`

- [ ] **Step 1: Add smoke test section**

Append after A/B Comparison Mode:

```markdown
## Smoke Test Mode (`--smoke`)

Automated non-interactive smoke test. Builds and launches the app,
runs predefined keystrokes, captures output after each, and checks
for panics or rendering failures.

### Build and launch

Same as single-branch mode. Use session name `smoke-<id>`.

### Test sequence

After the app starts (wait 3 seconds), run each test step. Between
every keystroke, wait 0.5 seconds and capture the pane. Each step
is one `tmux send-keys` call followed by one `capture-pane`.

**Tab navigation**: press `f` five times and `b` five times. Verify
each capture is non-empty and contains no "panic:" or
"runtime error".

**Dashboard overlay**: press `D` to open, capture, press `D` to
close, capture. Verify both captures are non-empty and differ from
each other.

**Row navigation**: press `j` three times, then `k` three times,
then `G` (end), then `g` (top). Verify captures are non-empty.

**House overlay**: press `Tab` to open, capture, press `Escape` to
close, capture.

**Detail view**: press `Enter` to open, capture, press `Escape` to
close, capture.

### Failure detection

After each capture, check for:
- Substring "panic:" or "runtime error" → FAIL
- Completely blank output (only whitespace) → FAIL
- App exited (shell prompt visible) → FAIL

### Report

Print a summary:

```
Smoke test: <label>
  [PASS/FAIL] Tab navigation
  [PASS/FAIL] Dashboard overlay
  [PASS/FAIL] Row navigation
  [PASS/FAIL] House overlay
  [PASS/FAIL] Detail view
  Result: N/5 passed
```

On failure, include the captured output that triggered the failure.

### Cleanup

Kill the tmux session after the smoke test completes:

```sh
tmux -L claude-tui kill-session -t smoke-<id>
```
```

- [ ] **Step 2: Commit**

Commit with: `docs: add smoke test mode to test-pr skill`

---

### Task 5: Manual verification

- [ ] **Step 1: Test single-branch mode**

Invoke `/test-pr 931` (or a current PR number). Verify:
- Binary builds successfully
- Database seeds
- tmux session created
- Handoff message printed with correct attach command

- [ ] **Step 2: Test A/B mode**

Invoke `/test-pr --ab 931`. Verify:
- Two binaries build
- Two databases seeded
- tmux session with two panes
- Both panes show the app running

- [ ] **Step 3: Test smoke mode**

Invoke `/test-pr 931 --smoke`. Verify:
- Automated keystrokes execute
- Report printed with pass/fail results
- Session cleaned up

- [ ] **Step 4: Test worktree name input**

Invoke `/test-pr agent-af040029`. Verify resolution works.

- [ ] **Step 5: Final commit if any fixes needed**
