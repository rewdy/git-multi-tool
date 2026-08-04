// Package gitutil provides small helpers for shelling out to git and
// resolving the kinds of commit references git-multi-tool commands accept:
// a single hash, a hash range ("abc123..def456"), or a count of recent
// commits ("-5").
package gitutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Run executes a git command in dir (or the current directory if dir is
// empty) and returns trimmed stdout. On failure it returns stderr wrapped
// in the error.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// RunInteractive executes a git command with the process's stdio attached,
// which is required for commands like rebase that may launch editors.
func RunInteractive(dir string, env []string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.Run()
}

// IsRepo reports whether dir (or cwd) is inside a git working tree.
func IsRepo(dir string) bool {
	out, err := Run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// TopLevel returns the root of the current git working tree.
func TopLevel(dir string) (string, error) {
	return Run(dir, "rev-parse", "--show-toplevel")
}

// CurrentBranch returns the checked-out branch name, or "" if detached.
func CurrentBranch(dir string) (string, error) {
	out, err := Run(dir, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return "", nil
	}
	return out, nil
}

// HasUncommittedChanges reports whether the working tree has staged or
// unstaged modifications.
func HasUncommittedChanges(dir string) (bool, error) {
	out, err := Run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// StatusLines returns the raw `git status --porcelain` lines, one per
// changed/untracked file, useful for previewing what a destructive
// operation is about to wipe out.
func StatusLines(dir string) ([]string, error) {
	out, err := Run(dir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// ResolveHash resolves any git revision expression to its full commit hash.
func ResolveHash(dir, rev string) (string, error) {
	return Run(dir, "rev-parse", rev)
}

// Commit is a lightweight summary of a single commit used for previews.
type Commit struct {
	Hash    string
	Short   string
	Author  string
	Email   string
	Subject string
}

// Target describes a contiguous span of commits to operate on, expressed
// as the commit immediately *before* the first commit to touch (Base) and
// the newest commit included (Head). Base may be empty, meaning the span
// starts at the repository's root commit.
type Target struct {
	Base string // exclusive lower bound, empty = repo root
	Head string // inclusive upper bound
}

// ParseSpec interprets a user-supplied spec string into a Target.
//
// Supported forms:
//   - "-N"                 the last N commits reachable from HEAD
//   - "<hash>"             that single commit
//   - "<hash>..<hash>"     commits after the first hash, up through the second
//   - "<hash>~N..<hash>"   any range syntax git itself understands
func ParseSpec(dir, spec string) (Target, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return Target{}, fmt.Errorf("empty commit spec")
	}

	if n, err := parseCount(spec); err == nil {
		head, herr := ResolveHash(dir, "HEAD")
		if herr != nil {
			return Target{}, herr
		}
		base, berr := nthParentOrRoot(dir, "HEAD", n)
		if berr != nil {
			return Target{}, berr
		}
		return Target{Base: base, Head: head}, nil
	}

	if strings.Contains(spec, "..") {
		parts := strings.SplitN(spec, "..", 2)
		baseRev, headRev := parts[0], parts[1]
		if baseRev == "" {
			return Target{}, fmt.Errorf("range %q is missing a starting commit", spec)
		}
		if headRev == "" {
			headRev = "HEAD"
		}
		base, err := ResolveHash(dir, baseRev)
		if err != nil {
			return Target{}, fmt.Errorf("can't resolve %q: %w", baseRev, err)
		}
		head, err := ResolveHash(dir, headRev)
		if err != nil {
			return Target{}, fmt.Errorf("can't resolve %q: %w", headRev, err)
		}
		return Target{Base: base, Head: head}, nil
	}

	head, err := ResolveHash(dir, spec)
	if err != nil {
		return Target{}, fmt.Errorf("can't resolve %q: %w", spec, err)
	}
	base, err := nthParentOrRoot(dir, spec, 1)
	if err != nil {
		return Target{}, err
	}
	return Target{Base: base, Head: head}, nil
}

func parseCount(spec string) (int, error) {
	if !strings.HasPrefix(spec, "-") {
		return 0, fmt.Errorf("not a count")
	}
	n, err := strconv.Atoi(spec[1:])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("not a count")
	}
	return n, nil
}

// nthParentOrRoot returns the hash N commits before rev, or "" if that
// walk would go past the root commit (in which case the whole history up
// to rev is included).
func nthParentOrRoot(dir, rev string, n int) (string, error) {
	out, err := Run(dir, "rev-parse", fmt.Sprintf("%s~%d", rev, n))
	if err != nil {
		// Walking past the root commit; there's simply no base.
		return "", nil
	}
	return out, nil
}

// ListCommits returns commits in t, oldest first, excluding Base and
// including Head.
func ListCommits(dir string, t Target) ([]Commit, error) {
	rangeArg := t.Head
	if t.Base != "" {
		rangeArg = t.Base + ".." + t.Head
	}
	out, err := Run(dir, "log", "--reverse", "--format=%H%x1f%h%x1f%an%x1f%ae%x1f%s", rangeArg)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	lines := strings.Split(out, "\n")
	commits := make([]Commit, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(line, "\x1f")
		if len(fields) != 5 {
			continue
		}
		commits = append(commits, Commit{
			Hash:    fields[0],
			Short:   fields[1],
			Author:  fields[2],
			Email:   fields[3],
			Subject: fields[4],
		})
	}
	return commits, nil
}

// Count returns how many commits are in t.
func Count(dir string, t Target) (int, error) {
	commits, err := ListCommits(dir, t)
	if err != nil {
		return 0, err
	}
	return len(commits), nil
}

// DefaultBranch tries to figure out the repository's "main" branch: the
// one the remote's HEAD points at (origin/HEAD), falling back to
// whichever of main/master actually exists locally if there's no remote
// configured (e.g. a fresh local-only repo).
func DefaultBranch(dir string) (string, error) {
	if out, err := Run(dir, "symbolic-ref", "--short", "-q", "refs/remotes/origin/HEAD"); err == nil && out != "" {
		return strings.TrimPrefix(out, "origin/"), nil
	}

	if out, err := Run(dir, "remote", "show", "origin"); err == nil {
		for line := range strings.SplitSeq(out, "\n") {
			line = strings.TrimSpace(line)
			if name, ok := strings.CutPrefix(line, "HEAD branch: "); ok {
				return name, nil
			}
		}
	}

	for _, candidate := range []string{"main", "master"} {
		if _, err := Run(dir, "show-ref", "--verify", "--quiet", "refs/heads/"+candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("couldn't figure out the default branch (no origin/HEAD, and no local main or master)")
}

// LocalBranches lists local branch names.
func LocalBranches(dir string) ([]string, error) {
	out, err := Run(dir, "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// DeleteBranch force-deletes a local branch.
func DeleteBranch(dir, name string) error {
	_, err := Run(dir, "branch", "-D", name)
	return err
}

// DeleteBranchIfMerged deletes a local branch with `git branch -d`, which
// refuses when the branch holds commits that aren't merged anywhere else.
// That refusal is the point: a branch whose remote vanished may still be
// the only copy of unpushed work, so callers surface the error rather than
// reaching for -D.
func DeleteBranchIfMerged(dir, name string) error {
	_, err := Run(dir, "branch", "-d", name)
	return err
}

// GoneBranch is a local branch whose upstream has disappeared, which is
// what's left behind after the remote branch is deleted (typically when a
// merge request merges).
type GoneBranch struct {
	Name     string // local branch name
	Upstream string // the tracking ref that no longer exists
}

// FetchPrune fetches and deletes remote-tracking refs whose upstream
// branches are gone, which is what makes GoneBranches able to see them.
// Streams git's own output, since this hits the network.
func FetchPrune(dir string) error {
	return RunInteractive(dir, nil, "fetch", "--prune")
}

// GoneBranches lists local branches whose upstream ref is gone. It asks
// for-each-ref for the same "[gone]" marker `git branch -vv` prints, but
// against a machine-readable format, so branch names containing spaces or
// upstreams named like the marker can't confuse the parse.
//
// Only branches that actually had an upstream can be gone, so branches
// that were never pushed are absent here rather than silently swept up.
func GoneBranches(dir string) ([]GoneBranch, error) {
	const sep = "\x1f"
	out, err := Run(dir, "for-each-ref",
		"--format=%(refname:short)"+sep+"%(upstream:short)"+sep+"%(upstream:track)",
		"refs/heads/")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var gone []GoneBranch
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Split(line, sep)
		if len(fields) != 3 || fields[2] != "[gone]" {
			continue
		}
		gone = append(gone, GoneBranch{Name: fields[0], Upstream: fields[1]})
	}
	return gone, nil
}

// StashPush stashes staged and unstaged changes (not untracked files)
// under the given message.
func StashPush(dir, message string) error {
	_, err := Run(dir, "stash", "push", "-m", message)
	return err
}

// StashPop pops the most recent stash.
func StashPop(dir string) error {
	_, err := Run(dir, "stash", "pop")
	return err
}

// HardResetAndClean discards all uncommitted changes (git reset --hard)
// and removes untracked files (git clean -f). It's destructive and
// irreversible; callers are expected to confirm with the user first.
func HardResetAndClean(dir string) error {
	if _, err := Run(dir, "reset", "--hard", "HEAD"); err != nil {
		return err
	}
	_, err := Run(dir, "clean", "-fd")
	return err
}

// Checkout switches to an existing local branch, streaming git's own
// output.
func Checkout(dir, branch string) error {
	return RunInteractive(dir, nil, "checkout", branch)
}

// Pull runs a plain git pull in dir, streaming git's own output.
func Pull(dir string) error {
	return RunInteractive(dir, nil, "pull")
}

// Fetch runs a plain git fetch in dir, streaming git's own output.
func Fetch(dir string) error {
	return RunInteractive(dir, nil, "fetch")
}

// RebaseOnto rebases the current branch onto upstream, streaming git's
// own output.
func RebaseOnto(dir, upstream string) error {
	return RunInteractive(dir, nil, "rebase", upstream)
}

// DiffStat returns a summary (`git diff --stat`) of what applying a
// snapshot restore from rev would change in the working tree, without
// changing anything.
func DiffStat(dir, rev string) (string, error) {
	return Run(dir, "diff", "HEAD", rev, "--stat")
}

// CheckSnapshotApplies reports whether restoring the working tree to rev
// (via ApplySnapshot) would succeed, without actually changing anything.
func CheckSnapshotApplies(dir, rev string) error {
	diff, err := diffPatch(dir, rev)
	if err != nil {
		return err
	}
	if diff == "" {
		return nil
	}
	cmd := exec.Command("git", "apply", "--check")
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin = strings.NewReader(diff)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git apply --check: %s", msg)
	}
	return nil
}

// ApplySnapshot rewrites the working tree to match rev's content,
// without touching commit history, the index, or moving HEAD. It's
// equivalent to `git diff HEAD <rev> | git apply`.
func ApplySnapshot(dir, rev string) error {
	diff, err := diffPatch(dir, rev)
	if err != nil {
		return err
	}
	if diff == "" {
		return nil
	}
	cmd := exec.Command("git", "apply")
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin = strings.NewReader(diff)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git apply: %s", msg)
	}
	return nil
}

func diffPatch(dir, rev string) (string, error) {
	cmd := exec.Command("git", "diff", "HEAD", rev)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git diff HEAD %s: %s", rev, msg)
	}
	return stdout.String(), nil
}
