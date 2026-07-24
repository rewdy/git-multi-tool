package cmd

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"git-multi-tool/internal/gitutil"
	"git-multi-tool/internal/style"
)

var backToMainFlags struct {
	stash bool
	clear bool
	yes   bool
}

var backToMainCmd = &cobra.Command{
	Use:     "back-to-main",
	Aliases: []string{"gbm", "back-to-master"},
	Short:   "Hop back to the default branch and delete the one you're leaving",
	Long: style.Heading("back-to-main") + `

Checks out the repo's default branch, pulls the latest changes, and
deletes the branch you were on. Handles any uncommitted changes first by
stashing or clearing them (your choice), and previews the full plan
before doing anything.`,
	RunE: runBackToMain,
}

func init() {
	f := backToMainCmd.Flags()
	f.BoolVar(&backToMainFlags.stash, "stash", false, "stash uncommitted changes instead of asking")
	f.BoolVar(&backToMainFlags.clear, "clear", false, "discard uncommitted changes instead of asking (same as running nuke first)")
	f.BoolVarP(&backToMainFlags.yes, "yes", "y", false, "skip the confirmation prompt")
}

func runBackToMain(cmd *cobra.Command, args []string) error {
	fmt.Println(style.Logo())
	fmt.Println()

	if backToMainFlags.stash && backToMainFlags.clear {
		return errors.New("--stash and --clear are mutually exclusive")
	}

	current, err := gitutil.CurrentBranch(repoDir)
	if err != nil {
		return err
	}
	if current == "" {
		return errors.New(style.ErrLine("you're in a detached HEAD state, nothing to leave or delete"))
	}

	main, err := gitutil.DefaultBranch(repoDir)
	if err != nil {
		fmt.Println(style.ErrLine("couldn't figure out the default branch: %v", err))
		return err
	}

	if current == main {
		fmt.Println(style.WarnLine("you're already on %s, nothing to do", main))
		return nil
	}

	dirty, err := gitutil.HasUncommittedChanges(repoDir)
	if err != nil {
		return err
	}

	fmt.Println(style.Heading("Here's the plan"))
	fmt.Println(style.Info.Render(fmt.Sprintf("  1. Check out %s", main)))
	fmt.Println(style.Info.Render("  2. Pull the latest changes"))
	fmt.Println(style.Danger.Render(fmt.Sprintf("  3. Delete branch %q", current)))
	if dirty {
		fmt.Println(style.Warning.Render("  + You've got uncommitted changes that need handling first"))
	}
	fmt.Println()

	action := ""
	switch {
	case backToMainFlags.stash:
		action = "stash"
	case backToMainFlags.clear:
		action = "clear"
	}

	if dirty && action == "" {
		if !isInteractive() {
			return errors.New("you have uncommitted changes; pass --stash or --clear to say what to do with them in a non-interactive session")
		}
		lines, err := gitutil.StatusLines(repoDir)
		if err != nil {
			return err
		}
		fmt.Println(style.Heading("Uncommitted changes"))
		fmt.Println(renderStatusList(lines))
		fmt.Println()

		err = huh.NewSelect[string]().
			Title("What do you want to do with them?").
			Options(
				huh.NewOption("Stash them (pop them back out later yourself)", "stash"),
				huh.NewOption("Clear them (git reset --hard && git clean -fd, no undo)", "clear"),
				huh.NewOption("Quit, I'll handle it myself", "quit"),
			).
			Value(&action).
			WithTheme(style.Theme()).
			Run()
		if err != nil {
			return err
		}
		if action == "quit" || action == "" {
			fmt.Println(style.WarnLine("okay, not doing anything"))
			return nil
		}
	}

	if !backToMainFlags.yes {
		if !isInteractive() {
			return errors.New("refusing to proceed without confirmation in a non-interactive session; pass --yes")
		}
		confirmed := false
		err := huh.NewConfirm().
			Title(fmt.Sprintf("Switch to %s and delete %q?", main, current)).
			Affirmative("Let's go").
			Negative("Not yet").
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

	if dirty {
		switch action {
		case "clear":
			fmt.Println(style.Info.Render("Clearing uncommitted changes..."))
			if err := gitutil.HardResetAndClean(repoDir); err != nil {
				fmt.Println(style.ErrLine("couldn't clear your changes: %v", err))
				return err
			}
			fmt.Println(style.SuccessLine("cleared"))
		case "stash":
			fmt.Println(style.Info.Render("Stashing uncommitted changes..."))
			if err := gitutil.StashPush(repoDir, "gmt back-to-main auto-stash"); err != nil {
				fmt.Println(style.ErrLine("couldn't stash your changes: %v", err))
				return err
			}
			fmt.Println(style.SuccessLine("stashed"))
		}
	}
	stashedForLater := dirty && action == "stash"

	fmt.Println(style.Info.Render(fmt.Sprintf("Checking out %s...", main)))
	if err := gitutil.Checkout(repoDir, main); err != nil {
		fmt.Println(style.ErrLine("checkout failed: %v", err))
		return err
	}
	fmt.Println(style.SuccessLine("checked out %s", main))
	fmt.Println()

	fmt.Println(style.Info.Render("Pulling latest changes..."))
	if err := gitutil.Pull(repoDir); err != nil {
		fmt.Println(style.ErrLine("pull failed: %v", err))
		return err
	}
	fmt.Println(style.SuccessLine("pulled latest and greatest"))
	fmt.Println()

	fmt.Println(style.Info.Render(fmt.Sprintf("Deleting branch %q...", current)))
	if err := gitutil.DeleteBranch(repoDir, current); err != nil {
		fmt.Println(style.ErrLine("couldn't delete %q: %v", current, err))
		return err
	}
	fmt.Println(style.SuccessLine("branch removed"))

	if stashedForLater {
		fmt.Println()
		fmt.Println(style.Info.Render("Applying your stashed changes..."))
		if err := gitutil.StashPop(repoDir); err != nil {
			fmt.Println(style.ErrLine("couldn't pop your stash, run `git stash pop` yourself: %v", err))
			return err
		}
		fmt.Println(style.SuccessLine("stash applied"))
	}

	fmt.Println()
	fmt.Println(style.SuccessLine("all done! you're on %s. 🎉", main))
	return nil
}
