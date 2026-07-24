// Package cmd wires up git-multi-tool's cobra command tree: the root
// command plus every maintenance subcommand.
package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"git-multi-tool/internal/gitutil"
	"git-multi-tool/internal/reauthor"
	"git-multi-tool/internal/style"
)

var repoDir string

const rootUse = "git-multi-tool"

var rootCmd = &cobra.Command{
	Use:           rootUse,
	Aliases:       []string{"gmt"},
	Short:         "A friendly forge for tidying up your git history",
	Long:          style.Logo() + "\n\ngit-multi-tool (gmt for short) is a growing toolbox of trivial-but-tedious git maintenance chores, wrapped in a TUI so you don't have to remember the incantations.",
	Version:       versionString(),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runMenu,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "__apply-reauthor-step" || cmd.Name() == rootUse {
			return nil
		}
		if !gitutil.IsRepo(repoDir) {
			return errors.New(style.ErrLine("this doesn't look like a git repository (checked %s)", displayDir()))
		}
		return nil
	},
}

func displayDir() string {
	if repoDir == "" {
		wd, err := os.Getwd()
		if err == nil {
			return wd
		}
		return "."
	}
	return repoDir
}

// Execute runs the git-multi-tool CLI.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&repoDir, "repo", "C", "", "path to the git repository (defaults to the current directory)")
	rootCmd.AddCommand(reauthorCmd)
	rootCmd.AddCommand(nukeCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(backToMainCmd)
	rootCmd.AddCommand(pruneBranchesCmd)
	rootCmd.AddCommand(restoreSnapshotCmd)
	rootCmd.AddCommand(applyReauthorStepCmd)
}

// applyReauthorStepCmd is a hidden command used internally as the --exec
// step of the rebase reauthor.Plan.Run drives. It is never meant to be
// invoked directly by a user.
var applyReauthorStepCmd = &cobra.Command{
	Use:    "__apply-reauthor-step",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return reauthor.RunApplyStep(repoDir)
	},
}
