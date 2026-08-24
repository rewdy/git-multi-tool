package cmd

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"git-multi-tool/internal/gitutil"
	"git-multi-tool/internal/style"
)

var syncFlags struct {
	stash bool
}

var syncCmd = &cobra.Command{
	Use:     "sync",
	Aliases: []string{"rebase-main", "gfr"},
	Short:   "Fetch and rebase your current branch onto the repo's default branch",
	Long: style.Heading("sync") + `

Fetches from origin and rebases your current branch onto the default
branch (whatever origin/HEAD or main/master resolves to). Refuses to run
if you're already on the default branch, just use ` + "`git pull --rebase`" + `
for that.`,
	RunE: runSync,
}

func init() {
	f := syncCmd.Flags()
	f.BoolVar(&syncFlags.stash, "stash", false, "stash uncommitted changes before rebasing, then pop them back after")
}

func runSync(cmd *cobra.Command, args []string) error {
	fmt.Println(style.Logo())
	fmt.Println()

	current, err := gitutil.CurrentBranch(repoDir)
	if err != nil {
		return err
	}
	if current == "" {
		return errors.New(style.ErrLine("you're in a detached HEAD state, checkout a branch first"))
	}

	main, err := gitutil.DefaultBranch(repoDir)
	if err != nil {
		fmt.Println(style.ErrLine("couldn't figure out the default branch: %v", err))
		return err
	}

	if current == main {
		fmt.Println(style.WarnLine("you're already on %s, just run `git pull --rebase` instead", main))
		return nil
	}

	dirty, err := gitutil.HasUncommittedChanges(repoDir)
	if err != nil {
		return err
	}

	if dirty && !syncFlags.stash && !cmd.Flags().Changed("stash") {
		if isInteractive() {
			wantStash := true
			err := huh.NewConfirm().
				Title("You've got uncommitted changes. Stash them for the rebase?").
				Description("They'll be popped back once the rebase finishes.").
				Affirmative("Stash 'em [y]").
				Negative("Leave them (may block the rebase) [n]").
				Value(&wantStash).
				WithTheme(style.Theme()).
				Run()
			if err != nil {
				return err
			}
			syncFlags.stash = wantStash
		}
	}

	fmt.Println(style.Info.Render(fmt.Sprintf("Rebasing %s onto %s...", current, main)))
	fmt.Println()

	stashed := false
	if dirty && syncFlags.stash {
		if err := gitutil.StashPush(repoDir, "gmt sync auto-stash"); err != nil {
			fmt.Println(style.ErrLine("couldn't stash your changes: %v", err))
			return err
		}
		stashed = true
		fmt.Println(style.SuccessLine("stashed uncommitted changes"))
	}

	restoreStash := func() {
		if stashed {
			if err := gitutil.StashPop(repoDir); err != nil {
				fmt.Println(style.ErrLine("couldn't pop your stash back, run `git stash pop` yourself: %v", err))
				return
			}
			fmt.Println(style.SuccessLine("popped your stashed changes back"))
		}
	}

	if err := gitutil.Fetch(repoDir); err != nil {
		fmt.Println(style.ErrLine("fetch failed: %v", err))
		restoreStash()
		return err
	}

	if err := gitutil.RebaseOnto(repoDir, "origin/"+main); err != nil {
		fmt.Println()
		fmt.Println(style.ErrLine("rebase hit a snag (repo left mid-rebase; run `git rebase --abort` to bail out, or resolve and `git rebase --continue`): %v", err))
		if stashed {
			fmt.Println(style.WarnLine("your stash is still there, run `git stash pop` once the rebase is sorted out"))
		}
		return err
	}

	restoreStash()

	fmt.Println()
	fmt.Println(style.SuccessLine("all done! %s is rebased onto %s.", current, main))
	return nil
}
