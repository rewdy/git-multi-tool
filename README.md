# ⚒ git-multi-tool (gmt)

A friendly forge for tidying up your git history.

`git-multi-tool` (`gmt` for short) is a growing toolbox of
trivial-but-tedious git maintenance chores, wrapped in a colorful little
TUI so you don't have to remember the incantations. Built with
[Cobra](https://github.com/spf13/cobra) for the CLI plumbing and Charm's
[huh](https://github.com/charmbracelet/huh) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss) for the pretty
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

## Building

```sh
go build -o gmt .
```

To bake in a real version (shown by `gmt --version`), pass it via
ldflags:

```sh
go build -ldflags "-X git-multi-tool/cmd.version=v1.2.3" -o gmt .
```

Without that flag, `--version` reports `dev`.

## Installing

```sh
go install .
```

This drops a `gmt` binary into `$(go env GOBIN)` (or `$(go env GOPATH)/bin`
if `GOBIN` isn't set), the command itself is also aliased as
`git-multi-tool` if you'd rather type the full name.

## Adding new commands

Each maintenance task lives as its own Cobra subcommand under `cmd/`,
backed by an engine package under `internal/`. Look at `cmd/reauthor.go`
and `internal/reauthor/reauthor.go` as the template: gather inputs with a
`huh` form (skipped automatically for flags/non-interactive use), preview
the blast radius, confirm, then execute.
