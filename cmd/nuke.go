package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"git-multi-tool/internal/gitutil"
	"git-multi-tool/internal/style"
)

var nukeFlags struct {
	yes bool
}

var nukeCmd = &cobra.Command{
	Use:     "nuke",
	Aliases: []string{"boom", "reset-hard"},
	Short:   "Blow away all uncommitted changes, tracked and untracked",
	Long: style.Heading("nuke") + `

Hard-resets tracked files back to HEAD and removes untracked files and
directories (` + "`git reset --hard HEAD && git clean -fd`" + `). This is
completely destructive and cannot be undone, gmt previews exactly what's
about to disappear before touching anything.`,
	RunE: runNuke,
}

func init() {
	f := nukeCmd.Flags()
	f.BoolVarP(&nukeFlags.yes, "yes", "y", false, "skip the confirmation prompt")
}

func runNuke(cmd *cobra.Command, args []string) error {
	fmt.Println(style.Logo())
	fmt.Println()

	lines, err := gitutil.StatusLines(repoDir)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		fmt.Println(style.SuccessLine("working tree is already spotless, nothing to nuke"))
		return nil
	}

	fmt.Println(style.Heading(fmt.Sprintf("%d file(s) are about to go 💥", len(lines))))
	fmt.Println(renderStatusList(lines))
	fmt.Println()
	fmt.Println(style.Muted.Render("This runs `git reset --hard HEAD && git clean -fd`. There is no undo."))
	fmt.Println()

	if !nukeFlags.yes {
		if !isInteractive() {
			return errors.New("refusing to nuke uncommitted changes without confirmation in a non-interactive session; pass --yes to proceed anyway")
		}
		confirmed := false
		err := huh.NewConfirm().
			Title("Really blow it all away?").
			Description("Tracked changes are reset and untracked files/dirs are deleted. No undo.").
			Affirmative("BOOM 💥").
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

	if err := gitutil.HardResetAndClean(repoDir); err != nil {
		fmt.Println(style.ErrLine("cleanup hit a snag: %v", err))
		return err
	}

	fmt.Println(style.SuccessLine("💥 all clean. working tree matches HEAD."))
	return nil
}

func renderStatusList(lines []string) string {
	styled := make([]string, len(lines))
	for i, l := range lines {
		styled[i] = style.Danger.Render("  ⚒ ") + style.Muted.Render(l)
	}
	return strings.Join(styled, "\n")
}
