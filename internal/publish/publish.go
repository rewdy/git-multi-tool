// Package publish pushes a branch to origin and, when asked, drives the
// server-side "open a merge request" action GitLab exposes through git push
// options. It has no UI: cmd/publish.go owns the prompting, the preview, and
// the confirmation gate.
//
// The merge-request half is GitLab-only. GitHub doesn't support push options
// for pull requests at all, so the command layer detects a GitHub remote and
// refuses up front rather than pushing and silently opening nothing.
package publish

import (
	"strings"

	"git-multi-tool/internal/gitutil"
)

// Host is which forge a remote points at, as far as we can tell from its
// URL. It decides whether merge-request push options are even possible.
type Host int

const (
	// HostUnknown is any remote we don't recognise. We still try the push
	// options, since a self-hosted GitLab won't have "gitlab" in its URL.
	HostUnknown Host = iota
	// HostGitLab is a gitlab.com (or obvious GitLab) remote.
	HostGitLab
	// HostGitHub is a github.com remote, where MR push options don't work.
	HostGitHub
)

// DetectHost guesses the forge behind a remote URL. It only special-cases
// GitHub with confidence (to fail fast) and the canonical gitlab.com; anything
// else is unknown, and callers optimistically attempt push options there
// because self-hosted GitLab instances live on arbitrary hostnames.
func DetectHost(remoteURL string) Host {
	u := strings.ToLower(remoteURL)
	switch {
	case strings.Contains(u, "github.com"):
		return HostGitHub
	case strings.Contains(u, "gitlab"):
		return HostGitLab
	default:
		return HostUnknown
	}
}

// Plan is a resolved publish: the branch to push, whether to open a merge
// request, and the MR's details when so. Title, Description, and Target are
// only consulted when CreateMR is true.
type Plan struct {
	Branch      string
	CreateMR    bool
	Target      string // MR target branch
	Title       string
	Description string

	// The optional server-side toggles GitLab understands.
	Draft                     bool
	RemoveSourceBranch        bool
	MergeWhenPipelineSucceeds bool
}

// PushOptions renders the plan into the `-o key=value` push options GitLab
// reads. It returns nil when CreateMR is false, so a plain publish pushes
// with no options at all.
func (p Plan) PushOptions() []string {
	if !p.CreateMR {
		return nil
	}
	opts := []string{"merge_request.create"}
	if t := strings.TrimSpace(p.Target); t != "" {
		opts = append(opts, "merge_request.target="+t)
	}
	if t := strings.TrimSpace(p.Title); t != "" {
		opts = append(opts, "merge_request.title="+t)
	}
	if d := strings.TrimSpace(p.Description); d != "" {
		opts = append(opts, "merge_request.description="+d)
	}
	if p.Draft {
		opts = append(opts, "merge_request.draft")
	}
	if p.RemoveSourceBranch {
		opts = append(opts, "merge_request.remove_source_branch")
	}
	if p.MergeWhenPipelineSucceeds {
		opts = append(opts, "merge_request.merge_when_pipeline_succeeds")
	}
	return opts
}

// Run pushes the branch, applying merge-request push options when the plan
// asks for one. It streams git's output.
func (p Plan) Run(dir string) error {
	return gitutil.Push(dir, p.Branch, p.PushOptions()...)
}
