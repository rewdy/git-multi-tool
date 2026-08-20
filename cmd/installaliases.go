package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"git-multi-tool/internal/shellsetup"
	"git-multi-tool/internal/style"
)

var installAliasesFlags struct {
	aliases []string
	all     bool
	shell   string
	rc      string
	bin     string
	yes     bool
	dryRun  bool
}

var installAliasesCmd = &cobra.Command{
	Use:     "install-aliases",
	Aliases: []string{"aliases"},
	Short:   "Add short shell aliases (gbm, gfr, boom, …) to your bash/zsh config",
	Long: style.Heading("install-aliases") + `

Wires git-multi-tool's short mnemonics up as real shell aliases so you can
type ` + "`gbm`" + ` instead of ` + "`git-multi-tool back-to-main`" + `. Run it
in a terminal to multi-select which aliases to add (ctrl+a toggles all), or
pass --all / --alias for scripted, non-interactive setup. The chosen aliases
are written into a managed block in your ~/.zshrc or ~/.bashrc, so re-running
updates them in place instead of duplicating.`,
	RunE: runInstallAliases,
}

func init() {
	f := installAliasesCmd.Flags()
	f.StringSliceVar(&installAliasesFlags.aliases, "alias", nil, "specific alias name(s) to install, e.g. --alias gbm --alias boom (repeatable)")
	f.BoolVar(&installAliasesFlags.all, "all", false, "install every available alias without prompting")
	f.StringVar(&installAliasesFlags.shell, "shell", "", "target shell: bash or zsh (defaults to autodetecting from $SHELL)")
	f.StringVar(&installAliasesFlags.rc, "rc", "", "path to the startup file to write (defaults to the shell's ~/.zshrc or ~/.bashrc)")
	f.StringVar(&installAliasesFlags.bin, "bin", "", "binary name the aliases invoke (defaults to this executable's name)")
	f.BoolVarP(&installAliasesFlags.yes, "yes", "y", false, "skip the confirmation prompt")
	f.BoolVar(&installAliasesFlags.dryRun, "dry-run", false, "show what would be written without touching any files")
}

func runInstallAliases(cmd *cobra.Command, args []string) error {
	fmt.Println(style.Logo())
	fmt.Println()

	sh, err := resolveShell(cmd)
	if err != nil {
		return err
	}

	rcPath := installAliasesFlags.rc
	if rcPath == "" {
		rcPath, err = shellsetup.RCPath(sh)
		if err != nil {
			return err
		}
	}

	bin := installAliasesFlags.bin
	if bin == "" {
		bin = resolveBinName()
	}

	selected, err := chooseAliases()
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Println(style.WarnLine("nothing selected, nothing to do"))
		return nil
	}

	fmt.Println(style.Heading(fmt.Sprintf("%d alias(es) will be added to %s", len(selected), rcPath)))
	for _, a := range selected {
		fmt.Println(style.Muted.Render("  ") + style.Info.Render(shellsetup.Line(a, bin)))
	}
	fmt.Println()

	if installAliasesFlags.dryRun {
		fmt.Println(style.WarnLine("dry run, nothing was written"))
		return nil
	}

	if !installAliasesFlags.yes {
		if !isInteractive() {
			return errors.New("refusing to modify your shell config without confirmation in a non-interactive session; pass --yes")
		}
		confirmed := false
		err := huh.NewConfirm().
			Title(fmt.Sprintf("Write these aliases into %s?", rcPath)).
			Affirmative("Do it").
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

	replaced, err := shellsetup.Install(rcPath, selected, bin)
	if err != nil {
		return err
	}

	if replaced {
		fmt.Println(style.SuccessLine("updated the git-multi-tool aliases in %s", rcPath))
	} else {
		fmt.Println(style.SuccessLine("added the git-multi-tool aliases to %s", rcPath))
	}
	fmt.Println(style.Muted.Render(fmt.Sprintf("Restart your shell or run `source %s` to start using them.", rcPath)))
	return nil
}

// resolveShell picks the target shell from --shell when set, otherwise
// autodetects from $SHELL.
func resolveShell(cmd *cobra.Command) (shellsetup.Shell, error) {
	if cmd.Flags().Changed("shell") {
		return shellsetup.ParseShell(installAliasesFlags.shell)
	}
	return shellsetup.DetectShell(), nil
}

// resolveBinName returns the name aliases should invoke: this executable's
// own base name, falling back to the canonical binary name.
func resolveBinName() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "git-multi-tool"
	}
	return filepath.Base(exe)
}

// chooseAliases resolves which aliases to install. Flags win so scripted use
// is never blocked on a prompt: --all takes everything, --alias picks named
// ones, and only a bare interactive invocation pops the multi-select.
func chooseAliases() ([]shellsetup.Alias, error) {
	catalog := shellsetup.Catalog()

	if installAliasesFlags.all {
		return catalog, nil
	}

	if len(installAliasesFlags.aliases) > 0 {
		out := make([]shellsetup.Alias, 0, len(installAliasesFlags.aliases))
		for _, name := range installAliasesFlags.aliases {
			a, ok := shellsetup.Find(name)
			if !ok {
				return nil, fmt.Errorf("unknown alias %q; run without flags to see the list, or use --all", name)
			}
			out = append(out, a)
		}
		return out, nil
	}

	if !isInteractive() {
		return nil, errors.New("no aliases specified: pass --all for every alias, --alias <name> for specific ones, or run in a terminal to pick interactively")
	}

	options := make([]huh.Option[string], 0, len(catalog))
	for _, a := range catalog {
		label := fmt.Sprintf("%s → %s — %s", a.Name, a.Subcommand, a.Summary)
		options = append(options, huh.NewOption(label, a.Name).Selected(true))
	}

	var picked []string
	err := huh.NewMultiSelect[string]().
		Title("Which aliases should gmt install?").
		Description("Everything's pre-selected; ctrl+a toggles all, space toggles one.").
		Options(options...).
		Value(&picked).
		WithTheme(style.Theme()).
		Run()
	if err != nil {
		return nil, err
	}

	out := make([]shellsetup.Alias, 0, len(picked))
	for _, name := range picked {
		if a, ok := shellsetup.Find(name); ok {
			out = append(out, a)
		}
	}
	return out, nil
}
