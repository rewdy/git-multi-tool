package cmd

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"git-multi-tool/internal/gitutil"
	"git-multi-tool/internal/style"
)

var pruneBranchesFlags struct {
	yes bool
	all bool
}

var pruneBranchesCmd = &cobra.Command{
	Use:     "prune-branches",
	Aliases: []string{"clear-branches"},
	Short:   "Pick local branches to delete, in bulk",
	Long: style.Heading("prune-branches") + `

Lists your local branches and lets you pick which ones to delete. The
current branch and the repo's default branch are never offered up, so
you can't accidentally delete the branch you're standing on or the one
everything else is based on.`,
	RunE: runPruneBranches,
}

func init() {
	f := pruneBranchesCmd.Flags()
	f.BoolVarP(&pruneBranchesFlags.yes, "yes", "y", false, "skip the confirmation prompt (only meaningful with --all)")
	f.BoolVar(&pruneBranchesFlags.all, "all", false, "delete every eligible branch without prompting for a selection")
}

func runPruneBranches(cmd *cobra.Command, args []string) error {
	fmt.Println(style.Logo())
	fmt.Println()

	current, err := gitutil.CurrentBranch(repoDir)
	if err != nil {
		return err
	}
	main, err := gitutil.DefaultBranch(repoDir)
	if err != nil {
		fmt.Println(style.WarnLine("couldn't figure out the default branch (%v), leaving it out of safety isn't guaranteed", err))
	}

	branches, err := gitutil.LocalBranches(repoDir)
	if err != nil {
		return err
	}

	eligible := make([]string, 0, len(branches))
	for _, b := range branches {
		if b == current || (main != "" && b == main) {
			continue
		}
		eligible = append(eligible, b)
	}

	if len(eligible) == 0 {
		fmt.Println(style.SuccessLine("nothing to prune, only %s and/or your current branch exist", main))
		return nil
	}

	var toDelete []string
	if pruneBranchesFlags.all {
		toDelete = eligible
	} else {
		if !isInteractive() {
			return errors.New("no branches specified: pass --all to delete every eligible branch, or run in a terminal to pick interactively")
		}
		options := make([]huh.Option[string], 0, len(eligible))
		for _, b := range eligible {
			options = append(options, huh.NewOption(b, b))
		}
		err := huh.NewMultiSelect[string]().
			Title("Which branches should gmt delete?").
			Description(fmt.Sprintf("%s and %s are excluded automatically.", current, orNone(main))).
			Options(options...).
			Value(&toDelete).
			WithTheme(style.Theme()).
			Run()
		if err != nil {
			return err
		}
	}

	if len(toDelete) == 0 {
		fmt.Println(style.WarnLine("nothing selected, nothing to do"))
		return nil
	}

	fmt.Println(style.Heading(fmt.Sprintf("%d branch(es) will be deleted", len(toDelete))))
	for _, b := range toDelete {
		fmt.Println(style.Danger.Render("  ⚒ ") + b)
	}
	fmt.Println()

	if !pruneBranchesFlags.yes {
		if !isInteractive() {
			return errors.New("refusing to delete branches without confirmation in a non-interactive session; pass --yes")
		}
		confirmed := false
		err := huh.NewConfirm().
			Title("Delete these branches?").
			Affirmative("Delete 'em [y]").
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

	failed := 0
	for _, b := range toDelete {
		if err := gitutil.DeleteBranch(repoDir, b); err != nil {
			fmt.Println(style.ErrLine("couldn't delete %q: %v", b, err))
			failed++
			continue
		}
		fmt.Println(style.SuccessLine("deleted %s", b))
	}

	if failed > 0 {
		return fmt.Errorf("%d branch(es) failed to delete", failed)
	}

	fmt.Println()
	fmt.Println(style.SuccessLine("all done! %d branch(es) gone.", len(toDelete)))
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "(no default branch detected)"
	}
	return s
}
