<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# `/test-pr` Skill

<!-- verified: 2026-04-14 -->

Skill for interactive testing of PR branches without leaving the
workspace. Builds the binary from a worktree, seeds a fresh database,
launches the app in a tmux session, and hands control to the user.

Tracks: to be filed as GitHub issue

## Usage

```
/test-pr 931                    # by PR number
/test-pr agent-af040029         # by worktree name
/test-pr --ab 931               # A/B: PR branch vs main
/test-pr --ab 931 933           # A/B: two PR branches
/test-pr 931 --smoke            # automated smoke test (non-interactive)
```

## Input Resolution

The skill accepts one of:

- **PR number** — looks up the PR's head branch via `gh pr view NNN
  --json headRefName`, then finds the matching worktree under
  `.claude/worktrees/` or checks out the branch into a temp worktree.
- **Worktree name** — resolved directly from `.claude/worktrees/<name>`.
- **Branch name** — matched against existing worktrees or checked out.

If no matching worktree exists and the branch is remote-only, the skill
creates a temporary worktree via `git worktree add`.

## Single-Branch Mode

### Build

```sh
direnv exec <worktree> bash -c \
  'cd <worktree> && go build -o /tmp/micasa-pr-<id> ./cmd/micasa'
```

Build via `go build` in the worktree's Nix devshell. The binary is
placed in `/tmp/` with a PR-specific name to avoid collisions.

### Database

Seed a fresh database via `demo --seed-only`:

```sh
/tmp/micasa-pr-<id> demo --seed-only /tmp/micasa-test-<id>.db
```

The database path uses the same `<id>` suffix as the binary. Previous
test databases at the same path are overwritten (clean slate).

### Launch

Create a tmux session and start the app:

```sh
tmux -L claude-tui new-session -d -s pr-<id> -x 120 -y 40
tmux -L claude-tui send-keys -t pr-<id> \
  "/tmp/micasa-pr-<id> /tmp/micasa-test-<id>.db" Enter
```

Uses the `claude-tui` socket (same as `/tui-testing`) to avoid
touching the user's tmux sessions.

### Handoff

Print instructions for the user to attach:

```
Built from worktree agent-af040029 (branch worktree-agent-af040029)
DB seeded at /tmp/micasa-test-931.db

Attach with:  ! tmux -L claude-tui attach -t pr-931

Detach with Ctrl+B, D to return here.
Kill session: ! tmux -L claude-tui kill-session -t pr-931
```

The `!` prefix runs the command in the Claude Code session so the user
gets a real interactive terminal.

## A/B Comparison Mode

When `--ab` is passed, the skill builds and launches two branches
side by side.

### Layout

Vertical split: left pane = first argument (PR branch), right pane =
second argument (main if only one PR specified).

```
┌──────────────────────┬──────────────────────┐
│   PR #931            │   main               │
│   (left pane)        │   (right pane)       │
│                      │                      │
└──────────────────────┴──────────────────────┘
```

### Build

Build two binaries:

```sh
# Left: PR branch
direnv exec <worktree-left> bash -c \
  'cd <worktree-left> && go build -o /tmp/micasa-ab-left ./cmd/micasa'

# Right: main (or second PR)
direnv exec <worktree-right> bash -c \
  'cd <worktree-right> && go build -o /tmp/micasa-ab-right ./cmd/micasa'
```

For main, build from the repository's main checkout directory (the
parent of `.claude/worktrees/`). This is the canonical main branch
working tree. If a second PR is specified, resolve its worktree the
same way as single-branch mode.

### Database

Separate databases for each side:

```sh
/tmp/micasa-ab-left demo --seed-only /tmp/micasa-ab-left.db
/tmp/micasa-ab-right demo --seed-only /tmp/micasa-ab-right.db
```

Same seed data in both so differences are purely from code changes.

### Launch

```sh
# Width is 2x single-pane (240) or terminal width, whichever is smaller.
tmux -L claude-tui new-session -d -s ab-<id> -x 240 -y 40
tmux -L claude-tui send-keys -t ab-<id> \
  "/tmp/micasa-ab-left /tmp/micasa-ab-left.db" Enter

tmux -L claude-tui split-window -h -t ab-<id>
tmux -L claude-tui send-keys -t ab-<id> \
  "/tmp/micasa-ab-right /tmp/micasa-ab-right.db" Enter
```

Width doubled to 240 so each pane gets ~120 columns.

### Handoff

```
A/B comparison: PR #931 (left) vs main (right)
DBs: /tmp/micasa-ab-left.db, /tmp/micasa-ab-right.db

Attach with:  ! tmux -L claude-tui attach -t ab-931

Switch panes: Ctrl+B, Arrow
Detach: Ctrl+B, D
Kill: ! tmux -L claude-tui kill-session -t ab-931
```

## Smoke Test Mode

When `--smoke` is passed, the skill runs an automated smoke test
instead of handing off to the user.

### Sequence

- Build and launch (same as single-branch mode)
- Wait 3 seconds for app startup
- Run predefined keystrokes with captures between each:
  - Navigate all tabs (b/f cycle)
  - Open dashboard overlay (D), close (D)
  - Navigate rows (j/k/g/G)
  - Open house overlay (Tab on header), close (Escape)
  - Open a detail view (Enter), close (Escape)
- After each capture, check for:
  - Panic output (substring match on "panic:", "runtime error")
  - Empty renders (blank pane)
  - Rendering artifacts (unmatched ANSI escapes, box-drawing breaks)
- Kill the session
- Report: pass/fail with captured evidence for any failures

### Output

```
Smoke test: PR #931
  [PASS] Tab navigation (5 tabs)
  [PASS] Dashboard overlay open/close
  [PASS] Row navigation
  [PASS] House overlay open/close
  [PASS] Detail view open/close
  Result: 5/5 passed
```

Or on failure:

```
  [FAIL] Dashboard overlay open/close
    Captured output contains "panic: runtime error: index out of range"
    [screen capture follows]
```

## Cleanup

The skill does NOT automatically clean up tmux sessions or temp files.
The user controls session lifetime. Cleanup commands are printed in the
handoff message.

Temp files (`/tmp/micasa-*`) are cleaned by the OS on reboot. Stale
tmux sessions from previous runs are killed at the start of each
`/test-pr` invocation (matching by session name).

## Skill File Location

`.claude/commands/test-pr.md` — standard Claude Code skill location.

## Dependencies

- `tmux` (available via Nix)
- `direnv` (available via Nix)
- `go` (available via Nix devshell)
- `gh` (for PR number resolution)
- `demo --seed-only` subcommand in micasa CLI
