package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/list"
	"github.com/spf13/cobra"

	"git-multi-tool/internal/style"
)

// runMenu is what happens when gmt is invoked with no subcommand: instead
// of dumping the usual cobra usage text, show off a friendly, browsable
// list of everything the toolbox can do.
func runMenu(cmd *cobra.Command, args []string) error {
	fmt.Println(style.Logo())
	fmt.Println()

	available := availableCommands(cmd.Root())
	if len(available) == 0 {
		fmt.Println(style.WarnLine("no commands are registered yet"))
		return nil
	}

	fmt.Println(style.Heading("What's in the toolbox"))
	fmt.Println(renderCommandList(available))
	fmt.Println()

	if !isInteractive() {
		fmt.Println(style.Muted.Render("Run `gmt <command> --help` for details, or `gmt --help` for global flags."))
		return nil
	}

	options := make([]huh.Option[string], 0, len(available))
	for _, sub := range available {
		options = append(options, huh.NewOption(sub.Name()+" — "+sub.Short, sub.Name()))
	}

	var pick string
	err := huh.NewSelect[string]().
		Title("Which command do you want to run?").
		Description("↑/↓ or j/k to move, enter to run, esc/q to quit").
		Options(options...).
		Value(&pick).
		WithTheme(style.Theme()).
		Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Println(style.Muted.Render("Suit yourself."))
			return nil
		}
		return err
	}

	root := cmd.Root()
	root.SetArgs([]string{pick})
	fmt.Println()
	return root.Execute()
}

// availableCommands returns every non-hidden, user-facing subcommand of
// root, skipping cobra's own built-ins (help, completion) so the toolbox
// listing only shows gmt's actual maintenance commands.
func availableCommands(root *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, sub := range root.Commands() {
		if !sub.IsAvailableCommand() {
			continue
		}
		if sub.Name() == "completion" {
			continue
		}
		out = append(out, sub)
	}
	return out
}

func renderCommandList(cmds []*cobra.Command) string {
	l := list.New().
		EnumeratorStyle(lipgloss.NewStyle().Foreground(style.Fuchsia).MarginRight(1)).
		ItemStyleFunc(func(_ list.Items, i int) lipgloss.Style {
			return lipgloss.NewStyle().Foreground(style.Gray)
		})

	for _, sub := range cmds {
		name := lipgloss.NewStyle().Bold(true).Foreground(style.Indigo).Render(sub.Name())
		aliases := ""
		if len(sub.Aliases) > 0 {
			aliases = style.Muted.Render(" (aka " + strings.Join(sub.Aliases, ", ") + ")")
		}
		l.Item(name + aliases + " — " + sub.Short)
	}
	return l.String()
}
