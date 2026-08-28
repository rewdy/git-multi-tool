// Package clone works out where a repository URL should land on disk and
// runs the clone. It has no UI: cmd/clone.go owns the prompting, the preview,
// and the print-the-path contract that lets a shell wrapper cd into the
// result.
package clone

import (
	"strconv"
	"strings"

	"git-multi-tool/internal/gitutil"
)

// Plan is a resolved clone: which repo, which directory it lands in, and the
// optional narrowing flags. Dir is always decided up front rather than left
// to git, because the caller has to print it afterwards.
type Plan struct {
	URL    string
	Dir    string
	Branch string // clone this branch instead of the remote's HEAD
	Depth  int    // shallow clone depth; 0 means full history
}

// DirName derives the directory name git itself would pick for a clone URL,
// which is the last path segment with any .git suffix dropped. It handles
// both real URLs (https://host/owner/repo.git) and scp-style remotes
// (git@host:owner/repo.git) by treating ':' as a separator too.
func DirName(url string) string {
	s := strings.TrimRight(strings.TrimSpace(url), "/")
	if i := strings.LastIndexAny(s, "/:"); i != -1 {
		s = s[i+1:]
	}
	return strings.TrimSuffix(s, ".git")
}

// Run performs the clone, streaming git's output.
func (p Plan) Run() error {
	var extra []string
	if p.Branch != "" {
		extra = append(extra, "--branch", p.Branch)
	}
	if p.Depth > 0 {
		extra = append(extra, "--depth", strconv.Itoa(p.Depth))
	}
	return gitutil.Clone(p.URL, p.Dir, extra...)
}
