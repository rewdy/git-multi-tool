// Package shellsetup turns git-multi-tool's short command mnemonics into
// real shell aliases and writes them into a user's bash or zsh startup
// file. It has no UI: cmd/installaliases.go owns all the prompting and
// rendering, this package just knows the alias catalog and how to splice a
// managed block into an rc file idempotently.
package shellsetup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Shell is one of the shells we know how to write aliases for. bash and zsh
// share identical `alias name='cmd'` syntax, so the only thing that differs
// between them is which startup file we target.
type Shell string

const (
	// Bash targets ~/.bashrc.
	Bash Shell = "bash"
	// Zsh targets ~/.zshrc.
	Zsh Shell = "zsh"
)

// Block markers delimit the region of the rc file we own. Everything between
// them is regenerated on each install so re-running the command updates the
// aliases in place instead of piling up duplicates.
const (
	blockStart = "# >>> git-multi-tool aliases >>>"
	blockEnd   = "# <<< git-multi-tool aliases <<<"
)

// Alias is one shell shortcut: a short mnemonic (Name) that expands to a
// git-multi-tool subcommand (Subcommand), with a human summary for previews.
type Alias struct {
	Name       string
	Subcommand string
	Summary    string
}

// Catalog is the curated set of shortcuts we offer: one memorable alias per
// user-facing command. The mnemonics that already exist as cobra aliases
// (gbm, gfr, boom, ggp) are kept identical so muscle memory carries over.
func Catalog() []Alias {
	return []Alias{
		{Name: "gbm", Subcommand: "back-to-main", Summary: "hop back to the default branch and delete the one you're leaving"},
		{Name: "gfr", Subcommand: "sync", Summary: "fetch and rebase your current branch onto the default branch"},
		{Name: "boom", Subcommand: "nuke", Summary: "blow away all uncommitted changes, tracked and untracked"},
		{Name: "ggp", Subcommand: "prune-gone", Summary: "fetch --prune, then delete local branches whose remote is gone"},
		{Name: "gpb", Subcommand: "prune-branches", Summary: "pick local branches to delete, in bulk"},
		{Name: "gra", Subcommand: "reauthor", Summary: "rewrite the author/committer identity on a run of commits"},
		{Name: "grs", Subcommand: "restore-snapshot", Summary: "make your working tree look like an old commit"},
	}
}

// Find returns the alias whose Name matches, and whether it was found.
func Find(name string) (Alias, bool) {
	for _, a := range Catalog() {
		if a.Name == name {
			return a, true
		}
	}
	return Alias{}, false
}

// DetectShell guesses the user's shell from $SHELL, falling back to zsh
// (the macOS default) when it can't tell.
func DetectShell() Shell {
	base := filepath.Base(os.Getenv("SHELL"))
	switch {
	case strings.Contains(base, "bash"):
		return Bash
	case strings.Contains(base, "zsh"):
		return Zsh
	default:
		return Zsh
	}
}

// ParseShell converts a user-supplied shell name into a Shell, rejecting
// anything we don't know how to write aliases for.
func ParseShell(s string) (Shell, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "bash":
		return Bash, nil
	case "zsh":
		return Zsh, nil
	default:
		return "", fmt.Errorf("unsupported shell %q (only bash and zsh are supported)", s)
	}
}

// RCPath returns the default startup file for a shell, anchored at the
// user's home directory.
func RCPath(sh Shell) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch sh {
	case Bash:
		return filepath.Join(home, ".bashrc"), nil
	case Zsh:
		return filepath.Join(home, ".zshrc"), nil
	default:
		return "", fmt.Errorf("unsupported shell %q", sh)
	}
}

// Line renders a single alias definition, e.g. alias gbm='git-multi-tool
// back-to-main'. bash and zsh share this syntax exactly.
func Line(a Alias, bin string) string {
	return fmt.Sprintf("alias %s='%s %s'", a.Name, bin, a.Subcommand)
}

// Block renders the full managed region we write into an rc file: the start
// marker, one alias line each, and the end marker.
func Block(aliases []Alias, bin string) string {
	var b strings.Builder
	b.WriteString(blockStart + "\n")
	for _, a := range aliases {
		b.WriteString(Line(a, bin) + "\n")
	}
	b.WriteString(blockEnd)
	return b.String()
}

// Install splices the managed block into the rc file at rcPath. If a block
// from a previous run is already present it's replaced in place; otherwise
// the block is appended. It reports whether the file already had a block
// (replaced) so callers can phrase their success message. The parent
// directory is created if needed so a brand-new shell config still works.
func Install(rcPath string, aliases []Alias, bin string) (replaced bool, err error) {
	block := Block(aliases, bin)

	existing, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(rcPath), 0o755); err != nil {
		return false, err
	}

	content := string(existing)
	updated, replaced := spliceBlock(content, block)

	if err := os.WriteFile(rcPath, []byte(updated), 0o644); err != nil {
		return false, err
	}
	return replaced, nil
}

// spliceBlock replaces an existing managed block in content with block, or
// appends it when none is present. It returns the new content and whether an
// existing block was replaced.
func spliceBlock(content, block string) (string, bool) {
	start := strings.Index(content, blockStart)
	end := strings.Index(content, blockEnd)
	if start != -1 && end != -1 && end > start {
		end += len(blockEnd)
		return content[:start] + block + content[end:], true
	}

	if content == "" {
		return block + "\n", false
	}
	sep := "\n"
	if !strings.HasSuffix(content, "\n") {
		sep = "\n\n"
	} else if !strings.HasSuffix(content, "\n\n") {
		sep = "\n"
	}
	return content + sep + block + "\n", false
}
