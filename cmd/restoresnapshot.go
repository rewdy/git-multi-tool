package cmd

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"git-multi-tool/internal/gitutil"
	"git-multi-tool/internal/style"
)

var restoreSnapshotFlags struct {
	rev    string
	stash  bool
	yes    bool
	dryRun bool
}

var restoreSnapshotCmd = &cobra.Command{
	Use:     "restore-snapshot",
	Aliases: []string{"snapshot-to"},
	Short:   "Make your working tree look like an old commit, without touching history",
	Long: style.Heading("restore-snapshot") + `

Rewrites the contents of your working tree to match an older commit,
without moving HEAD, touching the index, or altering history in any way
(equivalent to ` + "`git diff HEAD <rev> | git apply`" + `). Useful for
peeking at or recovering old file contents without a real checkout.

This is NOT the same as ` + "`git revert`" + `, nothing is committed, and
the change stays entirely in your working tree until you decide what to
do with it.`,
	RunE: runRestoreSnapshot,
}

func init() {
	f := restoreSnapshotCmd.Flags()
	f.StringVarP(&restoreSnapshotFlags.rev, "commit", "n", "", "the commit whose content you want to restore into the working tree")
	f.BoolVar(&restoreSnapshotFlags.stash, "stash", false, "stash your current uncommitted changes first, as a safety net")
	f.BoolVarP(&restoreSnapshotFlags.yes, "yes", "y", false, "skip the confirmation prompt")
	f.BoolVar(&restoreSnapshotFlags.dryRun, "dry-run", false, "show what would change without touching the working tree")
}

func runRestoreSnapshot(cmd *cobra.Command, args []string) error {
	fmt.Println(style.Logo())
	fmt.Println()

	if restoreSnapshotFlags.rev == "" && isInteractive() {
		err := huh.NewInput().
			Title("Which commit's content should gmt restore?").
			Description("Any git revision: a hash, a tag, HEAD~3, etc.").
			Value(&restoreSnapshotFlags.rev).
			Validate(func(s string) error {
				if s == "" {
					return errors.New("give gmt a commit to restore from")
				}
				return nil
			}).
			WithTheme(style.Theme()).
			Run()
		if err != nil {
			return err
		}
	}
	if restoreSnapshotFlags.rev == "" {
		return errors.New("no commit specified: pass -n/--commit, or run in a terminal for the interactive prompt")
	}

	rev, err := gitutil.ResolveHash(repoDir, restoreSnapshotFlags.rev)
	if err != nil {
		fmt.Println(style.ErrLine("couldn't resolve %q: %v", restoreSnapshotFlags.rev, err))
		return err
	}

	stat, err := gitutil.DiffStat(repoDir, rev)
	if err != nil {
		fmt.Println(style.ErrLine("couldn't compute what would change: %v", err))
		return err
	}
	if stat == "" {
		fmt.Println(style.SuccessLine("working tree already matches %s, nothing to do", style.ShortHash(rev)))
		return nil
	}

	fmt.Println(style.Heading(fmt.Sprintf("This is what would change to match %s", style.ShortHash(rev))))
	fmt.Println(style.Muted.Render(stat))
	fmt.Println()

	if err := gitutil.CheckSnapshotApplies(repoDir, rev); err != nil {
		fmt.Println(style.ErrLine("this snapshot won't apply cleanly on top of your current working tree: %v", err))
		fmt.Println(style.Muted.Render("Try committing, stashing, or discarding your current changes first."))
		return err
	}

	if restoreSnapshotFlags.dryRun {
		fmt.Println(style.SuccessLine("dry run complete, working tree untouched"))
		return nil
	}

	dirty, err := gitutil.HasUncommittedChanges(repoDir)
	if err != nil {
		return err
	}

	if dirty && !restoreSnapshotFlags.stash && !cmd.Flags().Changed("stash") {
		if isInteractive() {
			wantStash := true
			err := huh.NewConfirm().
				Title("You've got uncommitted changes. Stash them first as a safety net?").
				Description("They'll still be there afterward via `git stash pop`, this doesn't touch them, just protects them.").
				Affirmative("Stash 'em").
				Negative("Leave them be").
				Value(&wantStash).
				WithTheme(style.Theme()).
				Run()
			if err != nil {
				return err
			}
			restoreSnapshotFlags.stash = wantStash
		}
	}

	if !restoreSnapshotFlags.yes {
		if !isInteractive() {
			return errors.New("refusing to modify the working tree without confirmation in a non-interactive session; pass --yes")
		}
		confirmed := false
		err := huh.NewConfirm().
			Title(fmt.Sprintf("Restore working tree to match %s?", style.ShortHash(rev))).
			Description("This only changes tracked file contents in your working tree, no commits are made or moved.").
			Affirmative("Restore it").
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

	if dirty && restoreSnapshotFlags.stash {
		if err := gitutil.StashPush(repoDir, "gmt restore-snapshot auto-stash"); err != nil {
			fmt.Println(style.ErrLine("couldn't stash your changes: %v", err))
			return err
		}
		fmt.Println(style.SuccessLine("stashed your uncommitted changes (run `git stash pop` to bring them back)"))
	}

	if err := gitutil.ApplySnapshot(repoDir, rev); err != nil {
		fmt.Println(style.ErrLine("restore failed: %v", err))
		return err
	}

	fmt.Println(style.SuccessLine("working tree now matches %s", style.ShortHash(rev)))
	return nil
}
