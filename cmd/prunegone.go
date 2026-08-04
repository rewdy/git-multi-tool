package cmd

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"git-multi-tool/internal/gitutil"
	"git-multi-tool/internal/style"
)

var pruneGoneFlags struct {
	yes    bool
	dryRun bool
	force  bool
}

var pruneGoneCmd = &cobra.Command{
	Use:     "prune-gone",
	Aliases: []string{"ggp", "gone", "tidy"},
	Short:   "Fetch with --prune, then delete local branches whose remote is gone",
	Long: style.Heading("prune-gone") + `

Local branch maintenance after other people's merges land. Runs
` + "`git fetch --prune`" + ` to drop remote-tracking refs that no longer
exist upstream, then deletes the local branches those refs were tracking,
the ones ` + "`git branch -vv`" + ` marks ` + "`[gone]`" + `.

Deletion uses ` + "`git branch -d`" + `, never ` + "`-D`" + `, so a branch
still holding unmerged commits is refused by git and reported rather than
thrown away. Pass ` + "`--force`" + ` to override that once you've looked
at what it's complaining about.

Only branches that had an upstream can be gone, so branches you never
pushed are never candidates.`,
	RunE: runPruneGone,
}

func init() {
	f := pruneGoneCmd.Flags()
	f.BoolVarP(&pruneGoneFlags.yes, "yes", "y", false, "skip the confirmation prompt")
	f.BoolVar(&pruneGoneFlags.dryRun, "dry-run", false, "fetch and report what would be deleted without deleting anything")
	f.BoolVar(&pruneGoneFlags.force, "force", false, "delete with -D, even branches holding unmerged commits")
}

func runPruneGone(cmd *cobra.Command, args []string) error {
	fmt.Println(style.Logo())
	fmt.Println()

	// The gone list only exists after the prune, so the gate has to come
	// first and describe the operation rather than preview a branch list.
	// `git branch -d` is what keeps that honest: it refuses to drop
	// unmerged work, so consenting up front isn't consenting blindly.
	fmt.Println(style.Heading("Here's the plan"))
	fmt.Println(style.Info.Render("  1. git fetch --prune") +
		style.Muted.Render("   drop remote-tracking refs that are gone upstream"))
	fmt.Println(style.Info.Render("  2. delete the local branches that were tracking them"))
	fmt.Println()
	if pruneGoneFlags.force {
		fmt.Println(style.Danger.Render("  ⚒ --force is on: unmerged commits on those branches will be destroyed"))
	} else {
		fmt.Println(style.Muted.Render("  Deletes with `git branch -d`, so branches holding unmerged commits are kept and reported."))
	}
	fmt.Println()

	if !pruneGoneFlags.yes && !pruneGoneFlags.dryRun {
		if !isInteractive() {
			return errors.New("refusing to prune branches without confirmation in a non-interactive session; pass --yes")
		}
		confirmed := false
		if err := huh.NewConfirm().
			Title("Go ahead and tidy up?").
			Description("gmt will list what it found before deleting anything.").
			Affirmative("Tidy up").
			Negative("Not yet").
			Value(&confirmed).
			WithTheme(style.Theme()).
			Run(); err != nil {
			return err
		}
		if !confirmed {
			fmt.Println(style.WarnLine("cancelled, nothing was touched"))
			return nil
		}
	}

	fmt.Println(style.Info.Render("⚒ Fetching with --prune (git will report its own progress below)..."))
	fmt.Println()
	if err := gitutil.FetchPrune(repoDir); err != nil {
		fmt.Println(style.ErrLine("fetch --prune failed: %v", err))
		return err
	}

	gone, err := gitutil.GoneBranches(repoDir)
	if err != nil {
		return err
	}
	if len(gone) == 0 {
		fmt.Println()
		fmt.Println(style.SuccessLine("nothing to prune, no local branch has a gone upstream"))
		return nil
	}

	// A branch that's checked out can't be deleted, in this worktree or any
	// other, so leave it out of the list instead of letting git fail on it.
	current, err := gitutil.CurrentBranch(repoDir)
	if err != nil {
		return err
	}
	candidates := make([]gitutil.GoneBranch, 0, len(gone))
	for _, b := range gone {
		if b.Name == current {
			fmt.Println()
			fmt.Println(style.WarnLine("%s is gone upstream but it's the branch you're on, leaving it (switch away and re-run)", b.Name))
			continue
		}
		candidates = append(candidates, b)
	}

	if len(candidates) == 0 {
		fmt.Println()
		fmt.Println(style.SuccessLine("nothing left to prune once the branch you're on is out"))
		return nil
	}

	fmt.Println()
	fmt.Println(style.Heading(fmt.Sprintf("%d branch(es) have a gone upstream", len(candidates))))
	for _, b := range candidates {
		fmt.Println(style.Danger.Render("  ⚒ ") + b.Name + style.Muted.Render("   was tracking "+b.Upstream))
	}
	fmt.Println()

	if pruneGoneFlags.dryRun {
		fmt.Println(style.SuccessLine("dry run complete, the prune happened but no branches were deleted"))
		return nil
	}

	deleteBranch := gitutil.DeleteBranchIfMerged
	if pruneGoneFlags.force {
		deleteBranch = gitutil.DeleteBranch
	}

	deleted, kept, failed := 0, 0, 0
	for _, b := range candidates {
		if err := deleteBranch(repoDir, b.Name); err != nil {
			// Without --force the overwhelmingly likely cause is unmerged
			// commits, which is a deliberate refusal rather than a failure.
			if !pruneGoneFlags.force {
				fmt.Println(style.WarnLine("kept %s, git wouldn't delete it: %v", b.Name, err))
				kept++
				continue
			}
			fmt.Println(style.ErrLine("couldn't delete %s: %v", b.Name, err))
			failed++
			continue
		}
		fmt.Println(style.SuccessLine("deleted %s", b.Name))
		deleted++
	}

	if failed > 0 {
		return fmt.Errorf("%d branch(es) failed to delete", failed)
	}

	fmt.Println()
	if kept > 0 {
		fmt.Println(style.SuccessLine("all done! %d branch(es) gone, %d kept for holding unmerged work.", deleted, kept))
		fmt.Println(style.Muted.Render("Look those over, then re-run with --force if you're sure."))
		return nil
	}
	fmt.Println(style.SuccessLine("all done! %d branch(es) gone.", deleted))
	return nil
}
