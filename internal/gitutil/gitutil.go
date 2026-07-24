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
