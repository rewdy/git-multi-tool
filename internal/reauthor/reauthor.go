// Package reauthor implements git-multi-tool's commit author/committer
// rewrite engine. It rewrites history by replaying a non-interactive git
// rebase where each replayed commit is immediately amended (in a --exec
// step) with new identity fields, while carefully preserving
// author/committer dates and any field the caller doesn't want to touch.
package reauthor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"git-multi-tool/internal/gitutil"
)

// StepEnv is the name of the environment variable git-multi-tool uses to
// pass its rewrite configuration down into the hidden "apply" step that
// git invokes once per replayed commit during the rebase.
const StepEnv = "GMT_REAUTHOR_SPEC"

// Scope selects which commit identity fields a rewrite should touch.
type Scope struct {
	Author    bool
	Committer bool
}

// Rule is a single find-and-replace rule: commits whose current author
// email matches Match (case-insensitively; empty matches everything) get
// their name/email replaced by Name/Email (empty string leaves that field
// untouched).
type Rule struct {
	MatchEmail string
	MatchName  string
	Name       string
	Email      string
	Scope      Scope
}

// Encode serializes a Rule to the compact pipe-delimited form used to pass
// it through the environment to the hidden step command.
func (r Rule) Encode() string {
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	fields := []string{r.MatchEmail, r.MatchName, r.Name, r.Email, b(r.Scope.Author), b(r.Scope.Committer)}
	for i, f := range fields {
		fields[i] = strings.ReplaceAll(f, "|", "\\|")
	}
	return strings.Join(fields, "|")
}

// DecodeRule parses a Rule previously produced by Encode.
func DecodeRule(s string) (Rule, error) {
	parts := splitEscaped(s)
	if len(parts) != 6 {
		return Rule{}, fmt.Errorf("malformed reauthor rule: %q", s)
	}
	return Rule{
		MatchEmail: parts[0],
		MatchName:  parts[1],
		Name:       parts[2],
		Email:      parts[3],
		Scope: Scope{
			Author:    parts[4] == "1",
			Committer: parts[5] == "1",
		},
	}, nil
}

func splitEscaped(s string) []string {
	var out []string
	var cur strings.Builder
	esc := false
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case r == '\\':
			esc = true
		case r == '|':
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	out = append(out, cur.String())
	return out
}

func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// Plan is a fully-resolved rewrite ready to execute against a Target.
type Plan struct {
	Target gitutil.Target
	Rules  []Rule
}

// Preview reports which of the target's commits match at least one rule,
// useful for confirmation screens before rewriting anything.
func (p Plan) Preview(dir string) ([]gitutil.Commit, []bool, error) {
	commits, err := gitutil.ListCommits(dir, p.Target)
	if err != nil {
		return nil, nil, err
	}
	hits := make([]bool, len(commits))
	for i, c := range commits {
		hits[i] = matchesAny(p.Rules, c.Author, c.Email)
	}
	return commits, hits, nil
}

func matchesAny(rules []Rule, name, email string) bool {
	for _, r := range rules {
		if ruleMatches(r, name, email) {
			return true
		}
	}
	return false
}

func ruleMatches(r Rule, name, email string) bool {
	if r.MatchEmail != "" && !strings.EqualFold(r.MatchEmail, email) {
		return false
	}
	if r.MatchName != "" && !strings.EqualFold(r.MatchName, name) {
		return false
	}
	return true
}

// Run performs the rewrite. It replays exactly the commits in p.Target
// through a detached-HEAD rebase with an --exec step (git-multi-tool's own
// hidden "apply-step" command) that re-amends each replayed commit's
// identity according to the rules. If the target's head isn't the current
// branch's tip, any commits above it are replayed afterward unmodified
// via a second rebase --onto, so only commits inside the target range are
// ever touched. The original branch ref (if any) is updated to the final
// rewritten tip at the end.
func (p Plan) Run(dir string) error {
	if len(p.Rules) == 0 {
		return fmt.Errorf("no rewrite rules given")
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("can't locate git-multi-tool binary: %w", err)
	}

	var encoded []string
	for _, r := range p.Rules {
		encoded = append(encoded, r.Encode())
	}
	specEnv := StepEnv + "=" + strings.Join(encoded, "\x1e")
	env := append(os.Environ(), specEnv)

	branch, err := gitutil.CurrentBranch(dir)
	if err != nil {
		return err
	}
	origTip, err := gitutil.ResolveHash(dir, "HEAD")
	if err != nil {
		return err
	}

	abort := func() {
		_ = gitutil.RunInteractive(dir, nil, "rebase", "--abort")
		if branch != "" {
			_, _ = gitutil.Run(dir, "checkout", branch)
		}
	}

	if _, err := gitutil.Run(dir, "checkout", "--detach", p.Target.Head); err != nil {
		return fmt.Errorf("can't check out %s: %w", shortHash(p.Target.Head), err)
	}

	rebaseArgs := []string{"rebase"}
	if p.Target.Base == "" {
		rebaseArgs = append(rebaseArgs, "--root")
	} else {
		rebaseArgs = append(rebaseArgs, p.Target.Base)
	}
	rebaseArgs = append(rebaseArgs, "--exec", exe+" __apply-reauthor-step")

	if err := runGitWithEnv(dir, env, rebaseArgs...); err != nil {
		abort()
		return fmt.Errorf("rebase failed while rewriting the target range (repo restored to its original branch): %w", err)
	}

	newHead, err := gitutil.ResolveHash(dir, "HEAD")
	if err != nil {
		abort()
		return err
	}

	if origTip != p.Target.Head {
		if err := runGitWithEnv(dir, nil, "rebase", "--onto", newHead, p.Target.Head, origTip); err != nil {
			abort()
			return fmt.Errorf("rebase failed while replaying commits above the target range (repo restored to its original branch): %w", err)
		}
		newHead, err = gitutil.ResolveHash(dir, "HEAD")
		if err != nil {
			abort()
			return err
		}
	}

	if branch != "" {
		if _, err := gitutil.Run(dir, "update-ref", "refs/heads/"+branch, newHead); err != nil {
			return fmt.Errorf("rewrote history but couldn't update branch %q, it's sitting at detached HEAD %s: %w", branch, shortHash(newHead), err)
		}
		if _, err := gitutil.Run(dir, "checkout", branch); err != nil {
			return fmt.Errorf("rewrote history and updated %q, but couldn't check it back out: %w", branch, err)
		}
	}

	return nil
}

func runGitWithEnv(dir string, env []string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = env
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunApplyStep is the hidden step invoked by git (once per replayed
// commit) via `--exec`. It reads rules from StepEnv, checks whether HEAD's
// current identity matches any rule, and if so amends HEAD in place while
// preserving both author and committer dates, and any field not targeted
// by the matching rule's scope.
func RunApplyStep(dir string) error {
	raw := os.Getenv(StepEnv)
	if raw == "" {
		return fmt.Errorf("%s not set; this command is only meant to be invoked by git-multi-tool's rebase", StepEnv)
	}

	var rules []Rule
	for enc := range strings.SplitSeq(raw, "\x1e") {
		r, err := DecodeRule(enc)
		if err != nil {
			return err
		}
		rules = append(rules, r)
	}

	curName, err := gitutil.Run(dir, "log", "-1", "--format=%an")
	if err != nil {
		return err
	}
	curEmail, err := gitutil.Run(dir, "log", "-1", "--format=%ae")
	if err != nil {
		return err
	}

	var match *Rule
	for i := range rules {
		if ruleMatches(rules[i], curName, curEmail) {
			match = &rules[i]
			break
		}
	}
	if match == nil {
		return nil
	}

	authorName, err := gitutil.Run(dir, "log", "-1", "--format=%an")
	if err != nil {
		return err
	}
	authorEmail, err := gitutil.Run(dir, "log", "-1", "--format=%ae")
	if err != nil {
		return err
	}
	authorDate, err := gitutil.Run(dir, "log", "-1", "--format=%ad", "--date=raw")
	if err != nil {
		return err
	}
	committerName, err := gitutil.Run(dir, "log", "-1", "--format=%cn")
	if err != nil {
		return err
	}
	committerEmail, err := gitutil.Run(dir, "log", "-1", "--format=%ce")
	if err != nil {
		return err
	}
	committerDate, err := gitutil.Run(dir, "log", "-1", "--format=%cd", "--date=raw")
	if err != nil {
		return err
	}

	if match.Scope.Author {
		if match.Name != "" {
			authorName = match.Name
		}
		if match.Email != "" {
			authorEmail = match.Email
		}
	}
	if match.Scope.Committer {
		if match.Name != "" {
			committerName = match.Name
		}
		if match.Email != "" {
			committerEmail = match.Email
		}
	}

	cmd := exec.Command("git", "commit", "--amend", "--no-edit", "--allow-empty", "--reset-author")
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+authorName,
		"GIT_AUTHOR_EMAIL="+authorEmail,
		"GIT_AUTHOR_DATE="+authorDate,
		"GIT_COMMITTER_NAME="+committerName,
		"GIT_COMMITTER_EMAIL="+committerEmail,
		"GIT_COMMITTER_DATE="+committerDate,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
