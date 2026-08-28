package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"git-multi-tool/internal/clone"
	"git-multi-tool/internal/gitutil"
	"git-multi-tool/internal/style"
)

var cloneFlags struct {
	dir    string
	branch string
	depth  int
	dryRun bool
}

var cloneCmd = &cobra.Command{
	Use:     "clone [url]",
	Aliases: []string{"gcd"},
	Short:   "Clone a repo and land inside it, no follow-up cd required",
	Long: style.Heading("clone") + `

Clones a repository and prints the directory it landed in on stdout, so a
shell wrapper can cd you straight into it. Install that wrapper with
` + "`gmt install-aliases`" + ` (the ` + "`gcd`" + ` entry), then ` + "`gcd <url>`" + ` leaves
you standing in the fresh clone. Or capture the path yourself:
` + "`cd \"$(gmt clone <url>)\"`" + `.

Everything else it prints (banner, preview, git's progress) goes to stderr,
which is what keeps stdout a clean, machine-readable path.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runClone,
}

func init() {
	f := cloneCmd.Flags()
	f.StringVar(&cloneFlags.dir, "dir", "", "directory to clone into (defaults to the repo name from the URL)")
	f.StringVarP(&cloneFlags.branch, "branch", "b", "", "check out this branch instead of the remote's default")
	f.IntVar(&cloneFlags.depth, "depth", 0, "shallow clone with this many commits of history")
	f.BoolVar(&cloneFlags.dryRun, "dry-run", false, "show where it would clone to without cloning")
}

// promptable reports whether clone can pop up huh forms. It checks stderr
// rather than stdout (unlike isInteractive) because clone's stdout is usually
// a command substitution — the gcd wrapper — while huh renders to stderr
// regardless, so a piped stdout is no reason to skip the prompt.
func promptable() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stderr.Fd())
}

func runClone(cmd *cobra.Command, args []string) error {
	// out is every human-facing byte this command writes; stdout belongs to
	// the resulting path alone.
	out := os.Stderr

	fmt.Fprintln(out, style.Logo())
	fmt.Fprintln(out)

	url, err := resolveCloneURL(args)
	if err != nil {
		return err
	}
	if url == "" {
		fmt.Fprintln(out, style.WarnLine("no repo given, nothing to clone"))
		return nil
	}

	dest, err := resolveCloneDir(url)
	if err != nil {
		return err
	}

	// Re-running on a repo you already have should still put you in it,
	// since landing in the clone is the whole point of the command.
	if gitutil.IsRepo(dest) {
		fmt.Fprintln(out, style.WarnLine("%s is already a git repo, jumping straight in", dest))
		fmt.Println(dest)
		return nil
	}
	if err := checkCloneDest(dest); err != nil {
		return err
	}

	fmt.Fprintln(out, style.Heading("about to clone"))
	fmt.Fprintln(out, cloneField("from", url))
	fmt.Fprintln(out, cloneField("into", dest))
	if cloneFlags.branch != "" {
		fmt.Fprintln(out, cloneField("branch", cloneFlags.branch))
	}
	if cloneFlags.depth > 0 {
		fmt.Fprintln(out, cloneField("depth", fmt.Sprintf("%d", cloneFlags.depth)))
	}
	fmt.Fprintln(out)

	if cloneFlags.dryRun {
		fmt.Fprintln(out, style.WarnLine("dry run, nothing was cloned"))
		return nil
	}

	plan := clone.Plan{URL: url, Dir: dest, Branch: cloneFlags.branch, Depth: cloneFlags.depth}
	if err := plan.Run(); err != nil {
		return errors.New(style.ErrLine("clone failed: %v", err))
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, style.SuccessLine("cloned into %s", dest))
	// A tty on stdout means nobody's capturing the path, so the cd never
	// happened — worth mentioning the wrapper exists.
	if isatty.IsTerminal(os.Stdout.Fd()) {
		fmt.Fprintln(out, style.Muted.Render("Run `gmt install-aliases` and pick `gcd` to have your shell cd in for you."))
	}

	fmt.Println(dest)
	return nil
}

// resolveCloneURL takes the URL from the argument when given, otherwise asks
// for it. Args always win so scripted use is never blocked on a prompt.
func resolveCloneURL(args []string) (string, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return strings.TrimSpace(args[0]), nil
	}
	if !promptable() {
		return "", errors.New("no repository given: pass a clone URL as the first argument")
	}
	var url string
	err := huh.NewInput().
		Title("Which repo should I clone?").
		Placeholder("git@github.com:owner/repo.git").
		Value(&url).
		WithTheme(style.Theme()).
		Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(url), nil
}

// resolveCloneDir decides the absolute destination: --dir when set, otherwise
// the name git would have picked. Relative paths hang off -C/--repo when it
// was given, which is the only sensible reading of that flag here — there's no
// repository to point at yet, just a place to put one.
func resolveCloneDir(url string) (string, error) {
	dest := strings.TrimSpace(cloneFlags.dir)
	if dest == "" {
		dest = clone.DirName(url)
	}
	if dest == "" {
		return "", errors.New(style.ErrLine("couldn't work out a directory name from %q, pass --dir", url))
	}
	if !filepath.IsAbs(dest) && repoDir != "" {
		dest = filepath.Join(repoDir, dest)
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// checkCloneDest turns git's mid-clone "destination path already exists"
// failure into an up-front error. An existing but empty directory is fine,
// git clones into those happily.
func checkCloneDest(dest string) error {
	info, err := os.Stat(dest)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New(style.ErrLine("%s already exists and isn't a directory", dest))
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return errors.New(style.ErrLine("%s already exists and isn't empty", dest))
	}
	return nil
}

func cloneField(label, value string) string {
	return style.Muted.Render(fmt.Sprintf("  %-7s", label)) + style.Info.Render(value)
}
