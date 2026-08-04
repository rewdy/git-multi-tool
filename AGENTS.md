# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`git-multi-tool` (alias `gmt`) is a Go CLI that wraps tedious git maintenance chores in a Charm-flavored TUI. Cobra for CLI plumbing, `huh` for interactive forms, `lipgloss` for rendering.

## Commands

```sh
go build ./cmd/git-multi-tool          # build -> ./git-multi-tool (gitignored)
go vet ./...
go install ./cmd/git-multi-tool        # installs as `git-multi-tool` into $GOBIN

# bake in a version (otherwise --version reports "dev")
go build -ldflags "-X git-multi-tool/cmd.version=v1.2.3" ./cmd/git-multi-tool
```

There are no tests in the repo yet. Once added: `go test ./...`, single test via `go test ./internal/gitutil -run TestParseSpec`.

`gmt` is only ever a symlink to the installed binary (see README) — the module has one `main` package, in `cmd/git-multi-tool/`.

## Architecture

Three layers, strictly separated:

- **`cmd/git-multi-tool/main.go`** — the only `main`; calls `cmd.Execute()` and turns errors into exit code 1. It prints the error itself, which is why `rootCmd` sets `SilenceErrors`/`SilenceUsage`.
- **`cmd/`** (package `cmd`) — one file per subcommand, holding the Cobra definition, its flag struct, the `huh` prompts, the preview rendering, and the confirmation gate. This layer owns all user interaction.
- **`internal/`** — engines with no UI. `gitutil` is the only thing that shells out to git; `reauthor` is the history-rewrite engine; `submodule` is the submodule-refresh engine; `style` owns the palette, `huh` theme, and line renderers.

`cmd/root.go` is the wiring point: every subcommand must be added there in `init()`. The persistent `-C/--repo` flag lands in the package-level `repoDir`, which every command passes as the first arg to `gitutil`/engine functions — there is no config struct. `PersistentPreRunE` validates that `repoDir` is a git repo, exempting the root command (so the bare menu still works outside a repo) and the hidden `__apply-reauthor-step`.

### The command pattern

Every subcommand follows the same shape, and new ones should too:

1. Print `style.Logo()`.
2. Resolve state from `gitutil` (current branch, default branch, dirty tree).
3. Fill in missing inputs with a `huh` form — **only** when `isInteractive()`, and **only** for flags the user didn't set (`cmd.Flags().Changed("name")`). Flags always win so scripted use is never blocked on a prompt.
4. Preview the blast radius (a table, a status list, a `diff --stat`).
5. Confirm — and if `!isInteractive()` and no `--yes`, return an error rather than proceeding.
6. Execute via the engine, streaming git's own output for long operations.

`--dry-run` exits after step 4. Bail-out paths print a `style.WarnLine` and return `nil`, not an error — cancelling isn't a failure.

Shared `cmd` helpers live in whichever file first needed them, not a `common.go`: `isInteractive()` in `reauthor.go`, `renderStatusList()` in `nuke.go`, `truncate()` in `reauthor.go`, `isInteractive`'s consumers everywhere.

### `gitutil` conventions

`Run` captures stdout and wraps stderr into the error — use it for queries. `RunInteractive` attaches the process's stdio — required for anything that may launch an editor or wants git's own progress on screen (`rebase`, `pull`, `fetch`, `checkout`). A `dir` of `""` means "current directory".

`Target{Base, Head}` is the shared representation of a commit span: `Base` is the _exclusive_ lower bound and may be `""`, meaning "back to the root commit" (which is what makes `rebase --root` necessary). `ParseSpec` turns user input (`-5`, a hash, `a..b`) into one.

`DefaultBranch` tries `origin/HEAD`, then parses `git remote show origin`, then falls back to a local `main`/`master`.

Several helpers deliberately swallow git's non-zero exit and return a zero value instead, because the failure _is_ the answer: `CurrentBranch` → `""` when detached, `UpstreamRef` → `""` when there's no tracking ref, `SubmodulePaths` → `nil` when there's no `.gitmodules`. Don't "fix" these into errors.

### The submodule refresh strategy

`internal/submodule` exists because updating submodules isn't a loop over `git pull`. Submodules normally sit at a **detached HEAD**, where a plain pull fails outright ("You are not currently on a branch"), while `git submodule update --remote` works anywhere but always leaves them detached. So `State.Strategy` picks per submodule: `pull --ff-only` when it's on a branch _with_ an upstream, `--remote` when detached or on a branch that was never pushed.

`Strategy` is a method rather than a stored field so the preview and the executor can't disagree about what's about to happen — `cmd/submodulesupdate.go` renders from the same call the engine dispatches on.

Two things worth knowing before touching this: git already refuses to clobber uncommitted submodule work (it exits non-zero and leaves that submodule alone), so the dirty pre-flight check is about turning a _mid-run_ failure into an up-front decision, not about preventing data loss. And every successful update leaves the superproject with a modified gitlink — `ChangedGitlinks` asks git which ones actually moved rather than assuming, and `CommitPaths` uses `--only` so unrelated staged work isn't swept into the pointer commit.

`git submodule status` has no `--porcelain`; `SubmoduleStatus` parses its documented `-`/`+`/`U` prefixes. Note `Run` trims the whole output, so the first line loses its leading space while later ones keep theirs — the parser normalizes both.

### The reauthor rewrite mechanism

This is the one genuinely non-obvious piece. To change identities while preserving dates, messages, and content, `reauthor.Plan.Run`:

1. Detaches HEAD at `Target.Head`.
2. Runs a non-interactive `git rebase <Base> --exec "<own binary> __apply-reauthor-step"`, so git invokes gmt itself once per replayed commit.
3. Passes the rules to those child invocations by serializing them into the `GMT_REAUTHOR_SPEC` env var (`Rule.Encode`: `|`-delimited fields, backslash-escaped, multiple rules joined by `\x1e`).
4. `RunApplyStep` reads the env var, checks whether HEAD matches a rule, and if so `git commit --amend --reset-author` with all six `GIT_AUTHOR_*`/`GIT_COMMITTER_*` vars set explicitly — dates included, which is how they survive.
5. If commits existed above the target range, replays them with a second `rebase --onto`, then `update-ref`s the original branch to the new tip and checks it back out.

Consequences: the binary must be able to locate itself (`os.Executable()`), the hidden `__apply-reauthor-step` command must stay registered and exempt from the repo check, and any failure path calls `abort()` (`rebase --abort` + checkout of the original branch).

## Conventions

- Package comments on every package; doc comments on exported identifiers, written in plain prose about _why_, not just _what_.
- All user-facing output goes through `style` (`SuccessLine`, `ErrLine`, `WarnLine`, `Heading`, `Muted.Render`) — never bare `fmt.Println` of an unstyled message.
- Voice is casual and lowercase after the marker: `"cancelled, nothing was touched"`, `"working tree is already spotless, nothing to nuke"`.
- Destructive commands preview first, always. Every one supports `-y/--yes`; history/tree-mutating ones also support `--dry-run`.
- Give commands short aliases in `Aliases` (e.g. `sync` → `gfr`, `back-to-main` → `gbm`).
- `Long` descriptions start with `style.Heading("<name>")`.
- Every dependency in `go.mod` is currently annotated `// indirect` even though several are direct; a `go mod tidy` will rewrite those lines, so don't be surprised by that diff.

New commands: copy `cmd/reauthor.go` + `internal/reauthor/reauthor.go` as the template, register in `cmd/root.go`, and document in `README.md` (which is user-facing and kept in sync with the flag tables).
