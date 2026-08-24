package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"git-multi-tool/internal/gitutil"
	"git-multi-tool/internal/reauthor"
	"git-multi-tool/internal/style"
)

var reauthorFlags struct {
	spec        string
	matchEmail  string
	matchName   string
	newName     string
	newEmail    string
	scope       string
	filterByOld bool
	yes         bool
	dryRun      bool
}

var reauthorCmd = &cobra.Command{
	Use:     "reauthor",
	Aliases: []string{"rename-author", "author"},
	Short:   "Rewrite the author and/or committer identity on a run of commits",
	Long: style.Heading("reauthor") + `

Rewrite commit author and/or committer name & email across a hash, a hash
range, or the last N commits, without disturbing anything else (commit
dates, messages, and content all stay put).`,
	RunE: runReauthor,
}

func init() {
	f := reauthorCmd.Flags()
	f.StringVarP(&reauthorFlags.spec, "commits", "n", "", "which commits to rewrite: a hash, a hash..hash range, or -N for the last N commits")
	f.StringVar(&reauthorFlags.matchEmail, "match-email", "", "only rewrite commits whose current author email equals this (optional filter)")
	f.StringVar(&reauthorFlags.matchName, "match-name", "", "only rewrite commits whose current author name equals this (optional filter)")
	f.StringVar(&reauthorFlags.newName, "name", "", "new name to set")
	f.StringVar(&reauthorFlags.newEmail, "email", "", "new email to set")
	f.StringVar(&reauthorFlags.scope, "scope", "both", "identity fields to change: author, committer, or both")
	f.BoolVarP(&reauthorFlags.yes, "yes", "y", false, "skip the confirmation prompt")
	f.BoolVar(&reauthorFlags.dryRun, "dry-run", false, "show what would change without rewriting anything")
}

// isInteractive reports whether it's safe to pop up huh forms: we need a
// real terminal on both stdin and stdout.
func isInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

func runReauthor(cmd *cobra.Command, args []string) error {
	fmt.Println(style.Logo())
	fmt.Println()

	dirty, err := gitutil.HasUncommittedChanges(repoDir)
	if err != nil {
		return err
	}
	if dirty {
		fmt.Println(style.ErrLine("you've got uncommitted changes lying around. Commit, stash, or discard them before rewriting history."))
		return errors.New("uncommitted changes present")
	}

	if isInteractive() {
		if err := gatherReauthorInputs(cmd); err != nil {
			return err
		}
	}

	if strings.TrimSpace(reauthorFlags.spec) == "" {
		return errors.New("no commits specified: pass -n/--commits (e.g. -n -5), or run in a terminal for the interactive prompt")
	}
	if reauthorFlags.newName == "" && reauthorFlags.newEmail == "" {
		return errors.New("nothing to change: pass --name and/or --email")
	}

	target, err := gitutil.ParseSpec(repoDir, reauthorFlags.spec)
	if err != nil {
		fmt.Println(style.ErrLine("couldn't make sense of %q: %v", reauthorFlags.spec, err))
		return err
	}

	scope := reauthor.Scope{}
	switch strings.ToLower(reauthorFlags.scope) {
	case "author":
		scope.Author = true
	case "committer":
		scope.Committer = true
	default:
		scope.Author = true
		scope.Committer = true
	}

	rule := reauthor.Rule{
		MatchEmail: reauthorFlags.matchEmail,
		MatchName:  reauthorFlags.matchName,
		Name:       reauthorFlags.newName,
		Email:      reauthorFlags.newEmail,
		Scope:      scope,
	}
	plan := reauthor.Plan{Target: target, Rules: []reauthor.Rule{rule}}

	commits, hits, err := plan.Preview(repoDir)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		fmt.Println(style.WarnLine("no commits found in that range, nothing to do"))
		return nil
	}

	fmt.Println(style.Heading("Here's the blast radius"))
	fmt.Println(renderPreviewTable(commits, hits))
	fmt.Println()

	matchCount := 0
	for _, h := range hits {
		if h {
			matchCount++
		}
	}
	if matchCount == 0 {
		fmt.Println(style.WarnLine("no commits in that range match your filters, nothing to rewrite"))
		return nil
	}

	fmt.Println(style.Info.Render(fmt.Sprintf("%d of %d commit(s) will be rewritten.", matchCount, len(commits))))
	fmt.Println(style.Muted.Render("Rewriting history changes commit hashes. If this branch is shared, you'll need to force-push and give collaborators a heads up."))
	fmt.Println()

	if reauthorFlags.dryRun {
		fmt.Println(style.SuccessLine("dry run complete, no history was touched"))
		return nil
	}

	if !reauthorFlags.yes {
		if !isInteractive() {
			return errors.New("refusing to rewrite history without confirmation in a non-interactive session; pass --yes to proceed anyway")
		}
		confirmed := false
		err := huh.NewConfirm().
			Title("Ready to swing the hammer?").
			Description("This rewrites commits in place via a git rebase.").
			Affirmative("Let's forge it ⚒ [y]").
			Negative("Not yet [n]").
			Value(&confirmed).
			WithTheme(style.Theme()).
			Run()
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println(style.WarnLine("cancelled, nothing was touched"))
			return nil
		}
	}

	fmt.Println(style.Info.Render("⚒ Reforging commit history (git will report its own progress below)..."))
	fmt.Println()

	if err := plan.Run(repoDir); err != nil {
		fmt.Println()
		fmt.Println(style.ErrLine("the rebase hit a snag: %v", err))
		return err
	}

	fmt.Println(style.SuccessLine("history rewritten! %d commit(s) now carry their new identity.", matchCount))
	return nil
}

// gatherReauthorInputs fills in any reauthorFlags fields the user didn't
// supply via flags by asking for them with a huh form. Flags always win,
// so scripted / non-interactive use never gets interrupted by prompts.
func gatherReauthorInputs(cmd *cobra.Command) error {
	changed := cmd.Flags().Changed
	var groups []*huh.Group

	if !changed("commits") {
		groups = append(groups, huh.NewGroup(
			huh.NewInput().
				Title("Which commits should gmt touch?").
				Description("A single hash, a hash..hash range, or -N for the last N commits (e.g. -5).").
				Placeholder("-5").
				Value(&reauthorFlags.spec).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("give gmt something to work with")
					}
					return nil
				}),
		))
	}

	if !changed("name") && !changed("email") {
		groups = append(groups, huh.NewGroup(
			huh.NewInput().
				Title("New name").
				Description("Leave blank to keep each commit's existing name.").
				Value(&reauthorFlags.newName),
			huh.NewInput().
				Title("New email").
				Description("Leave blank to keep each commit's existing email. At least one of name/email is required.").
				Value(&reauthorFlags.newEmail).
				Validate(func(s string) error {
					if s == "" && reauthorFlags.newName == "" {
						return errors.New("set at least a new name or a new email")
					}
					return nil
				}),
		))
	}

	if !changed("scope") {
		groups = append(groups, huh.NewGroup(
			huh.NewSelect[string]().
				Title("What should change?").
				Options(
					huh.NewOption("Author + committer (the usual choice)", "both"),
					huh.NewOption("Author only", "author"),
					huh.NewOption("Committer only", "committer"),
				).
				Value(&reauthorFlags.scope),
		))
	}

	if !changed("match-email") && !changed("match-name") {
		reauthorFlags.filterByOld = true
		groups = append(groups, huh.NewGroup(
			huh.NewConfirm().
				Title("Only rewrite commits matching a specific current email?").
				Description("Say no to rewrite every commit in the range, no questions asked.").
				Affirmative("Filter it [y]").
				Negative("Rewrite all [n]").
				Value(&reauthorFlags.filterByOld),
		))
		groups = append(groups, huh.NewGroup(
			huh.NewInput().
				Title("Current email to match").
				Description("Only commits whose author email exactly matches this get rewritten.").
				Value(&reauthorFlags.matchEmail),
		).WithHideFunc(func() bool {
			return !reauthorFlags.filterByOld
		}))
	}

	if len(groups) == 0 {
		return nil
	}

	form := huh.NewForm(groups...).WithTheme(style.Theme())
	if err := form.Run(); err != nil {
		return err
	}
	if !reauthorFlags.filterByOld {
		reauthorFlags.matchEmail = ""
	}
	return nil
}

func renderPreviewTable(commits []gitutil.Commit, hits []bool) string {
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(style.Indigo)).
		Headers("", "Hash", "Author", "Email", "Subject").
		StyleFunc(func(row, col int) lipgloss.Style {
			base := lipgloss.NewStyle().Padding(0, 1)
			if row == table.HeaderRow {
				return base.Bold(true).Foreground(style.Indigo)
			}
			if row >= 0 && row < len(hits) && hits[row] {
				return base.Foreground(style.Mint)
			}
			return base.Foreground(style.Gray)
		})

	for i, c := range commits {
		marker := " "
		if hits[i] {
			marker = "⚒"
		}
		t.Row(marker, c.Short, c.Author, c.Email, truncate(c.Subject, 40))
	}
	return t.Render()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
