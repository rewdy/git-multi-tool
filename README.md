# ⚒ git-multi-tool (gmt)

A friendly forge for tidying up your git history.

`git-multi-tool` is a growing toolbox of trivial-but-tedious git
maintenance chores, wrapped in a colorful little TUI so you don't have
to remember the incantations. The examples below use its short alias,
`gmt`, see [Setting up the `gmt` shortcut](#setting-up-the-gmt-shortcut)
to configure it. Built with [Cobra](https://github.com/spf13/cobra) for
the CLI plumbing and Charm's [huh](https://github.com/charmbracelet/huh)
and [Lip Gloss](https://github.com/charmbracelet/lipgloss) for the pretty
bits.

## Commands

### Bare `gmt`

Run `gmt` with no arguments and it pops open a browsable list of every
maintenance command in the toolbox instead of the usual cobra usage
dump. In an interactive terminal you can pick one to see its `--help`
right there; piped/non-interactive sessions just get the list.

```sh
gmt
```

### `reauthor`

Rewrite the author and/or committer name/email across a run of commits,
without disturbing anything else, commit dates, messages, and content
all stay exactly where they are.

```sh
gmt reauthor
```

Run it bare and it'll ask you everything it needs to know. Or drive it
entirely with flags for scripting:

```sh
gmt reauthor \
  --commits -5 \
  --name "Ada Lovelace" \
  --email ada@example.com \
  --match-email old-address@example.com \
  --scope both \
  --yes
```

Flags:

| Flag | Description |
| --- | --- |
| `-n, --commits` | Which commits to touch: a single hash, a `hash..hash` range, or `-N` for the last N commits |
| `--name` | New name to set (leave blank to keep each commit's existing name) |
| `--email` | New email to set (leave blank to keep each commit's existing email) |
| `--match-email` | Only rewrite commits whose *current* author email equals this |
| `--match-name` | Only rewrite commits whose *current* author name equals this |
| `--scope` | `author`, `committer`, or `both` (default) |
| `--dry-run` | Show what would change without touching anything |
| `-y, --yes` | Skip the confirmation prompt |
| `-C, --repo` | Path to the git repo (defaults to the current directory) |

`reauthor` refuses to run with a dirty working tree, previews exactly
which commits will change before doing anything, and only ever rewrites
commits inside the range you asked for, everything above that range is
replayed untouched. Because it rewrites history, commit hashes change;
if you're working on a shared branch you'll need to force-push and give
your collaborators a heads-up.

### `nuke`

```sh
gmt nuke
```

Runs `git reset --hard HEAD && git clean -fd`, wiping out every tracked
and untracked change in the working tree. Previews exactly which files
are about to disappear and asks for confirmation first (`-y/--yes` to
skip it). Completely destructive, there is no undo.

### `sync`

```sh
gmt sync [--stash]
```

Fetches from `origin` and rebases your current branch onto the repo's
default branch (whatever `origin/HEAD`, or a local `main`/`master`,
resolves to). Refuses to run if you're already on the default branch.
Pass `--stash` to automatically stash uncommitted changes before
rebasing and pop them back afterward; without it you'll be prompted
interactively if you have any. If the rebase conflicts, it leaves the
repo mid-rebase with git's usual `--abort`/`--continue` escape hatches.

### `back-to-main`

```sh
gmt back-to-main
```

The button for "I'm done with this branch." Checks out the repo's
default branch, pulls the latest changes, and deletes the branch you
were on. Shows the full plan (checkout, pull, delete) before doing
anything, and if you've got uncommitted changes it asks whether to
stash (`--stash`) or discard (`--clear`) them first. Refuses to run if
you're already on the default branch or in a detached HEAD state.

### `prune-branches`

```sh
gmt prune-branches
```

Lets you multi-select which local branches to delete in bulk. Your
current branch and the repo's default branch are automatically excluded
from the list, so you can't accidentally delete either. Pass `--all` to
delete every eligible branch without the picker (still asks for
confirmation unless you also pass `-y/--yes`).

### `prune-gone`

```sh
gmt prune-gone
```

Local branch maintenance for after everyone else's merges land. Runs
`git fetch --prune` to drop remote-tracking refs that no longer exist
upstream, then deletes the local branches that were tracking them, the
ones `git branch -vv` marks `[gone]`. Since that list only exists after
the prune, the confirmation prompt comes first and spells out both steps
rather than previewing branch names.

Flags:

| Flag | Description |
| --- | --- |
| `--force` | Delete with `-D`, even branches holding unmerged commits |
| `--dry-run` | Fetch and report what would be deleted, without deleting |
| `-y, --yes` | Skip the confirmation prompt |
| `-C, --repo` | Path to the git repo (defaults to the current directory) |

Deletion uses `git branch -d`, never `-D`, so a branch whose remote is
gone but which still holds unmerged commits is refused by git and
reported instead of thrown away, which matters because a deleted
upstream doesn't guarantee the work got merged. `--force` overrides that
once you've read the list. The branch you're standing on is left alone
even when its upstream is gone, and branches you never pushed are never
candidates at all, only a branch that had an upstream can have lost one.

### `restore-snapshot`

```sh
gmt restore-snapshot -n <commit>
```

Rewrites your working tree's file contents to match an older commit,
without moving HEAD, touching the index, or altering history in any way
(it's `git diff HEAD <commit> | git apply` under the hood). Handy for
peeking at or recovering old file contents without a real checkout. Not
the same as `git revert`, nothing gets committed. Shows a `diff --stat`
preview and validates the patch applies cleanly (`git apply --check`)
before touching anything; if your working tree is dirty it offers to
stash first as a safety net. Supports `--dry-run`.

## Building

```sh
go build ./cmd/git-multi-tool
```

To bake in a real version (shown by `git-multi-tool --version`), pass it
via ldflags:

```sh
go build -ldflags "-X git-multi-tool/cmd.version=v1.2.3" ./cmd/git-multi-tool
```

Without that flag, `--version` reports `dev`.

## Installing

```sh
go install ./cmd/git-multi-tool
```

Go names the installed binary after its package directory, so this
always produces a `git-multi-tool` binary in `$(go env GOBIN)` (or
`$(go env GOPATH)/bin` if `GOBIN` isn't set). Make sure that directory is
on your `PATH`.

### Setting up the `gmt` shortcut

The binary is intentionally installed under its full, unambiguous name.
For the short `gmt` alias used throughout this README, symlink it once
after installing (re-run this any time you reinstall/upgrade, since
`go install` overwrites the target but not the symlink):

```sh
ln -sf "$(go env GOPATH)/bin/git-multi-tool" "$(go env GOPATH)/bin/gmt"
```

If your shell resolves commands via `GOBIN` instead, substitute
`$(go env GOBIN)` above. Either way, `gmt` and `git-multi-tool` will then
be the exact same binary, just two names for it, no wrapper scripts or
shell functions required.

## Adding new commands

Each maintenance task lives as its own Cobra subcommand under `cmd/`,
backed by an engine package under `internal/`. Look at `cmd/reauthor.go`
and `internal/reauthor/reauthor.go` as the template: gather inputs with a
`huh` form (skipped automatically for flags/non-interactive use), preview
the blast radius, confirm, then execute. The actual `main` package lives
in `cmd/git-multi-tool/`, so the built/installed binary is always named
`git-multi-tool`.
