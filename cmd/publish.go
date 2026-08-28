package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"git-multi-tool/internal/gitutil"
	"git-multi-tool/internal/publish"
	"git-multi-tool/internal/style"
)

var publishFlags struct {
	createMR     bool
	target       string
	title        string
	description  string
	draft        bool
	removeSource bool
	mergeOnPipe  bool
	yes          bool
	dryRun       bool
}

var publishCmd = &cobra.Command{
	Use:     "publish",
	Aliases: []string{"pub", "gpub"},
	Short:   "Push your branch to origin, optionally opening a merge request in one go",
	Long: style.Heading("publish") + `

Pushes the current branch to origin and sets its upstream. With
` + "`-c/--create-mr`" + ` it also opens a GitLab merge request in the same push,
via git push options, prefilled from a form (defaulted to your latest
commit's subject and body).

Merge-request creation is GitLab-only: GitHub doesn't support push options
for pull requests, so ` + "`-c`" + ` against a GitHub remote fails fast rather than
pushing and opening nothing. A plain ` + "`publish`" + ` works anywhere.`,
	RunE: runPublish,
}

func init() {
	f := publishCmd.Flags()
	f.BoolVarP(&publishFlags.createMR, "create-mr", "c", false, "open a GitLab merge request as part of the push")
	f.StringVarP(&publishFlags.target, "target", "t", "", "merge request target branch (defaults to the repo's default branch)")
	f.StringVar(&publishFlags.title, "title", "", "merge request title (defaults to your latest commit subject)")
	f.StringVarP(&publishFlags.description, "description", "d", "", "merge request description (defaults to your latest commit body)")
	f.BoolVar(&publishFlags.draft, "draft", false, "mark the merge request as a draft")
	f.BoolVar(&publishFlags.removeSource, "remove-source-branch", false, "delete the source branch once the merge request merges")
	f.BoolVar(&publishFlags.mergeOnPipe, "merge-when-pipeline-succeeds", false, "auto-merge the request once its pipeline passes")
	f.BoolVarP(&publishFlags.yes, "yes", "y", false, "skip the confirmation prompt")
	f.BoolVar(&publishFlags.dryRun, "dry-run", false, "show what would be pushed without pushing")
}

func runPublish(cmd *cobra.Command, args []string) error {
	fmt.Println(style.Logo())
	fmt.Println()

	branch, err := gitutil.CurrentBranch(repoDir)
	if err != nil {
		return err
	}
	if branch == "" {
		return errors.New(style.ErrLine("you're in a detached HEAD state, checkout a branch first"))
	}

	// A merge request needs a GitLab remote. Detect the host up front so a
	// GitHub `-c` fails before we push, rather than pushing and quietly
	// opening nothing.
	if publishFlags.createMR {
		remoteURL, err := gitutil.RemoteURL(repoDir, "origin")
		if err != nil {
			return err
		}
		if remoteURL == "" {
			return errors.New(style.ErrLine("no `origin` remote is configured, so there's nowhere to open a merge request"))
		}
		if publish.DetectHost(remoteURL) == publish.HostGitHub {
			fmt.Println(style.ErrLine("opening a merge request from a push isn't supported on GitHub."))
			fmt.Println(style.Muted.Render("GitHub has no push option for pull requests. Push with a plain `gmt publish`, then open the PR with `gh pr create` or the web UI."))
			return errors.New("merge request creation unsupported on GitHub")
		}
	}

	if publishFlags.createMR {
		if publishFlags.target == "" && !cmd.Flags().Changed("target") {
			if main, err := gitutil.DefaultBranch(repoDir); err == nil {
				publishFlags.target = main
			}
		}
		if isInteractive() {
			if err := gatherPublishInputs(cmd); err != nil {
				return err
			}
		}
	}

	plan := publish.Plan{
		Branch:                    branch,
		CreateMR:                  publishFlags.createMR,
		Target:                    publishFlags.target,
		Title:                     publishFlags.title,
		Description:               publishFlags.description,
		Draft:                     publishFlags.draft,
		RemoveSourceBranch:        publishFlags.removeSource,
		MergeWhenPipelineSucceeds: publishFlags.mergeOnPipe,
	}

	fmt.Println(style.Heading("about to publish"))
	fmt.Println(publishField("branch", branch))
	fmt.Println(publishField("remote", "origin"))
	if plan.CreateMR {
		fmt.Println(publishField("merge req", "yes"))
		if plan.Target != "" {
			fmt.Println(publishField("target", plan.Target))
		}
		if title := strings.TrimSpace(plan.Title); title != "" {
			fmt.Println(publishField("title", truncate(title, 60)))
		}
		if opts := mrToggleSummary(plan); opts != "" {
			fmt.Println(publishField("options", opts))
		}
	} else {
		fmt.Println(publishField("merge req", "no (plain push)"))
	}
	fmt.Println()

	if publishFlags.dryRun {
		fmt.Println(style.WarnLine("dry run, nothing was pushed"))
		return nil
	}

	if !publishFlags.yes {
		if !isInteractive() {
			return errors.New("refusing to push without confirmation in a non-interactive session; pass --yes to proceed anyway")
		}
		confirmed := true
		err := huh.NewConfirm().
			Title("Push it?").
			Description(publishConfirmDescription(plan)).
			Affirmative("Ship it 🚀 [y]").
			Negative("Not yet [n]").
			Value(&confirmed).
			WithTheme(style.Theme()).
			Run()
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println(style.WarnLine("cancelled, nothing was pushed"))
			return nil
		}
	}

	fmt.Println(style.Info.Render("Pushing to origin (git will report its own progress below)..."))
	fmt.Println()

	if err := plan.Run(repoDir); err != nil {
		fmt.Println()
		fmt.Println(style.ErrLine("push failed: %v", err))
		return err
	}

	fmt.Println()
	if plan.CreateMR {
		fmt.Println(style.SuccessLine("pushed %s and requested a merge request, check git's output above for the link.", branch))
	} else {
		fmt.Println(style.SuccessLine("pushed %s to origin.", branch))
	}
	return nil
}

// gatherPublishInputs fills the MR title and description the user didn't set
// via flags, defaulting to the latest commit's subject and body, which mirrors
// how GitLab prefills a single-commit merge request. It also offers the
// server-side toggles as a checklist. Flags always win, so scripted use is
// never interrupted.
func gatherPublishInputs(cmd *cobra.Command) error {
	changed := cmd.Flags().Changed
	var groups []*huh.Group

	if !changed("title") {
		if subject, err := gitutil.LatestCommitSubject(repoDir); err == nil {
			publishFlags.title = subject
		}
	}
	if !changed("description") {
		if body, err := gitutil.LatestCommitBody(repoDir); err == nil {
			publishFlags.description = body
		}
	}

	if !changed("title") || !changed("description") {
		title := huh.NewInput().
			Title("Merge request title").
			Description("Defaults to your latest commit subject.").
			Value(&publishFlags.title).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return errors.New("a merge request needs a title")
				}
				return nil
			})
		desc := huh.NewText().
			Title("Merge request description").
			Description("Defaults to your latest commit body. Leave blank for none.").
			Value(&publishFlags.description)
		fields := []huh.Field{}
		if !changed("title") {
			fields = append(fields, title)
		}
		if !changed("description") {
			fields = append(fields, desc)
		}
		groups = append(groups, huh.NewGroup(fields...))
	}

	if !changed("draft") && !changed("remove-source-branch") && !changed("merge-when-pipeline-succeeds") {
		var picked []string
		groups = append(groups, huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Any of these? (space to toggle)").
				Options(
					huh.NewOption("Mark as draft", "draft"),
					huh.NewOption("Remove source branch when merged", "remove-source-branch"),
					huh.NewOption("Merge when pipeline succeeds", "merge-when-pipeline-succeeds"),
				).
				Value(&picked),
		))
		// The multiselect writes into picked; fold the result back after the
		// form runs, below.
		defer func() {
			for _, p := range picked {
				switch p {
				case "draft":
					publishFlags.draft = true
				case "remove-source-branch":
					publishFlags.removeSource = true
				case "merge-when-pipeline-succeeds":
					publishFlags.mergeOnPipe = true
				}
			}
		}()
	}

	if len(groups) == 0 {
		return nil
	}

	form := huh.NewForm(groups...).WithTheme(style.Theme())
	return form.Run()
}

func mrToggleSummary(p publish.Plan) string {
	var on []string
	if p.Draft {
		on = append(on, "draft")
	}
	if p.RemoveSourceBranch {
		on = append(on, "remove source branch")
	}
	if p.MergeWhenPipelineSucceeds {
		on = append(on, "merge when pipeline succeeds")
	}
	return strings.Join(on, ", ")
}

func publishConfirmDescription(p publish.Plan) string {
	if p.CreateMR {
		if p.Target != "" {
			return fmt.Sprintf("Pushes %s to origin and opens a merge request into %s.", p.Branch, p.Target)
		}
		return fmt.Sprintf("Pushes %s to origin and opens a merge request.", p.Branch)
	}
	return fmt.Sprintf("Pushes %s to origin and sets its upstream.", p.Branch)
}

func publishField(label, value string) string {
	return style.Muted.Render(fmt.Sprintf("  %-11s", label)) + style.Info.Render(value)
}
