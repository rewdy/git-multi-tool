package cmd

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"

	"github.com/rewdy/git-multi-tool/internal/gitutil"
	"github.com/rewdy/git-multi-tool/internal/style"
)

var recentBranchesFlags struct {
	limit int
}

var recentBranchesCmd = &cobra.Command{
	Use:     "recent-branches",
	Aliases: []string{"recent", "glb"},
	Short:   "List local branches, most recently committed first",
	Long: style.Heading("recent-branches") + `

Lists your local branches sorted by their latest commit, newest first,
with the date and time of that commit. Handy for finding the branch you
were on last week. Read-only, nothing gets touched.`,
	RunE: runRecentBranches,
}

func init() {
	recentBranchesCmd.Flags().IntVarP(&recentBranchesFlags.limit, "limit", "n", 10, "how many branches to show (0 for all)")
}

func runRecentBranches(cmd *cobra.Command, args []string) error {
	fmt.Println(style.Logo())
	fmt.Println()

	if recentBranchesFlags.limit < 0 {
		return errors.New("--limit can't be negative")
	}

	current, err := gitutil.CurrentBranch(repoDir)
	if err != nil {
		return err
	}
	branches, err := gitutil.RecentBranches(repoDir, recentBranchesFlags.limit)
	if err != nil {
		return err
	}
	if len(branches) == 0 {
		fmt.Println(style.WarnLine("no local branches yet, nothing to list"))
		return nil
	}

	fmt.Println(style.Heading(fmt.Sprintf("%d most recent branch(es)", len(branches))))
	fmt.Println(renderRecentBranches(branches, current))
	return nil
}

func renderRecentBranches(branches []gitutil.RecentBranch, current string) string {
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(style.Indigo)).
		Headers("", "Branch", "Last commit", "", "Subject").
		StyleFunc(func(row, col int) lipgloss.Style {
			base := lipgloss.NewStyle().Padding(0, 1)
			if row == table.HeaderRow {
				return base.Bold(true).Foreground(style.Indigo)
			}
			if row >= 0 && row < len(branches) && branches[row].Name == current {
				return base.Foreground(style.Mint)
			}
			return base.Foreground(style.Gray)
		})

	for _, b := range branches {
		marker := " "
		if b.Name == current {
			marker = "*"
		}
		t.Row(marker, b.Name, b.Date, b.Relative, truncate(b.Subject, 40))
	}
	return t.Render()
}
