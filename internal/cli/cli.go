// Package cli implements the rag-init command line.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/maple52046/rag-init-cli/internal/content"
	"github.com/maple52046/rag-init-cli/internal/scaffold"
	"github.com/maple52046/rag-init-cli/internal/tools"
)

// Env is everything the CLI touches outside its own code.
type Env struct {
	In     io.Reader
	Out    io.Writer
	Err    io.Writer
	IsTTY  bool
	Runner tools.Runner
	// Load returns the rag-init content for --ref and --from.
	Load func(ref, from string) (*content.Bundle, error)
	// Version is the CLI version, shown by `rag-init version`.
	Version string
}

// errUsage marks errors caused by wrong arguments (exit code 2).
var errUsage = errors.New("usage error")

const usage = `rag-init sets up a knowledge base that coding agents read and write.

Usage:
  rag-init new <dir> [flags]     create a new knowledge-base repository
  rag-init init [dir] [flags]    add rag-init to an existing project (default: current directory)
  rag-init version               print the CLI version and the embedded rag-init version
  rag-init help                  show this help

Flags for new and init:
  --agent <name>    hand over to this agent afterwards: claude, codex, cursor, antigravity, gemini, or none
  --gemini          make Gemini CLI read AGENTS.md (.gemini/settings.json)
  --ref <ref>       download this rag-init tag, branch, or commit instead of the embedded version
  --from <dir>      use a local rag-init checkout instead of the embedded version
  --dry-run         show what would change and stop
  -y, --yes         do not ask for confirmation

Flags for new only:
  --github <repo>   also create the GitHub repository (owner/name) with the GitHub CLI and push
  --public          make the GitHub repository public (default: private)
  --no-git          do not run git init and the first commit

Flags for init only:
  --with-sources    also copy the methodology sources (knowledge/sources/, THIRD_PARTY_NOTICES.md)
  --update-skills   replace the create-knowledge-rules and git-commit skills if they already exist

Existing files are never overwritten: init only adds missing files, appends the knowledge-rules
section to an AGENTS.md that lacks it, and adds @AGENTS.md to CLAUDE.md.
`

// Run executes the command line and returns the process exit code.
func Run(args []string, env Env) int {
	err := run(args, env)
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errUsage):
		fmt.Fprintf(env.Err, "rag-init: %v\nRun `rag-init help` for usage.\n", err)
		return 2
	default:
		fmt.Fprintf(env.Err, "rag-init: %v\n", err)
		return 1
	}
}

func run(args []string, env Env) error {
	if len(args) == 0 {
		fmt.Fprint(env.Out, usage)
		return nil
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(env.Out, usage)
		return nil
	case "version", "--version":
		fmt.Fprintf(env.Out, "rag-init %s\nembedded rag-init content: %s\n", env.Version, content.EmbeddedRef())
		return nil
	case "new":
		return runNew(args[1:], env)
	case "init":
		return runInit(args[1:], env)
	default:
		return fmt.Errorf("%w: unknown command %q", errUsage, args[0])
	}
}

type common struct {
	agent  string
	gemini bool
	ref    string
	from   string
	dryRun bool
	yes    bool
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.agent, "agent", "", "")
	fs.BoolVar(&c.gemini, "gemini", false, "")
	fs.StringVar(&c.ref, "ref", "", "")
	fs.StringVar(&c.from, "from", "", "")
	fs.BoolVar(&c.dryRun, "dry-run", false, "")
	fs.BoolVar(&c.yes, "yes", false, "")
	fs.BoolVar(&c.yes, "y", false, "")
}

func (c *common) validate() error {
	if c.ref != "" && c.from != "" {
		return fmt.Errorf("%w: use either --ref or --from, not both", errUsage)
	}
	if c.agent != "" && c.agent != "none" {
		if _, ok := tools.AgentByKey(c.agent); !ok {
			return fmt.Errorf("%w: unknown agent %q (choose %s, or none)", errUsage, c.agent, strings.Join(tools.AgentKeys(), ", "))
		}
	}
	return nil
}

func runNew(args []string, env Env) error {
	fs := newFlagSet("new")
	var c common
	c.register(fs)
	github := fs.String("github", "", "")
	public := fs.Bool("public", false, "")
	noGit := fs.Bool("no-git", false, "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("%w: new needs exactly one directory", errUsage)
	}
	if err := c.validate(); err != nil {
		return err
	}
	if *github != "" && *noGit {
		return fmt.Errorf("%w: --github needs git; drop --no-git", errUsage)
	}
	dir := pos[0]

	b, err := env.Load(c.ref, c.from)
	if err != nil {
		return err
	}
	plan, err := scaffold.PlanNew(b, dir, scaffold.Options{Gemini: c.gemini})
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "rag-init will create %s from %s:\n", dir, b.Source)
	printPlan(env.Out, plan)
	if !*noGit {
		fmt.Fprintln(env.Out, "then    git init on branch main and commit every file")
	}
	if *github != "" {
		vis := "private"
		if *public {
			vis = "public"
		}
		fmt.Fprintf(env.Out, "then    create the %s GitHub repository %s and push\n", vis, *github)
	}
	if c.dryRun {
		return nil
	}
	if err := confirm(env, c.yes); err != nil {
		return err
	}
	if err := plan.Apply(); err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "Created %s.\n", dir)

	if !*noGit {
		if err := tools.GitInit(env.Runner, dir, "chore: start knowledge base from "+b.Source); err != nil {
			fmt.Fprintf(env.Err, "warning: %v\nThe files are in place; commit them yourself.\n", err)
			if *github != "" {
				fmt.Fprintln(env.Err, "warning: skipped creating the GitHub repository because the first commit failed.")
				*github = ""
			}
		} else {
			fmt.Fprintln(env.Out, "Committed the files on branch main.")
		}
	}
	if *github != "" {
		if err := tools.CreateGitHubRepo(env.Runner, dir, *github, !*public); err != nil {
			fmt.Fprintf(env.Err, "warning: %v\n", err)
		} else {
			fmt.Fprintf(env.Out, "Created the GitHub repository %s and pushed.\n", *github)
		}
	}
	return handoff(env, dir, c.agent)
}

func runInit(args []string, env Env) error {
	fs := newFlagSet("init")
	var c common
	c.register(fs)
	withSources := fs.Bool("with-sources", false, "")
	updateSkills := fs.Bool("update-skills", false, "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 1 {
		return fmt.Errorf("%w: init takes at most one directory", errUsage)
	}
	if err := c.validate(); err != nil {
		return err
	}
	dir := "."
	if len(pos) == 1 {
		dir = pos[0]
	}

	b, err := env.Load(c.ref, c.from)
	if err != nil {
		return err
	}
	plan, err := scaffold.PlanInit(b, dir, scaffold.Options{
		WithSources:  *withSources,
		UpdateSkills: *updateSkills,
		Gemini:       c.gemini,
	})
	if err != nil {
		return err
	}
	abs, _ := filepath.Abs(dir)
	fmt.Fprintf(env.Out, "rag-init will update %s from %s:\n", abs, b.Source)
	printPlan(env.Out, plan)
	if c.dryRun {
		return nil
	}
	if !plan.Changes() {
		fmt.Fprintln(env.Out, "Nothing to change.")
		return handoff(env, dir, c.agent)
	}
	if err := confirm(env, c.yes); err != nil {
		return err
	}
	if err := plan.Apply(); err != nil {
		return err
	}
	fmt.Fprintln(env.Out, "Done. Review the changes and commit them.")
	return handoff(env, dir, c.agent)
}

// handoff offers to start a coding agent that runs create-knowledge-rules.
func handoff(env Env, dir, choice string) error {
	if choice == "none" {
		return nil
	}
	manual := fmt.Sprintf("To fit the rules to your situation, open %s in your agent and ask it to run the create-knowledge-rules skill.", dir)
	var picked tools.Installed
	switch {
	case choice != "":
		a, _ := tools.AgentByKey(choice)
		bin, ok := tools.Find(env.Runner, a)
		if !ok {
			fmt.Fprintf(env.Err, "warning: %s is not installed (looked for %s).\n%s\n", a.Name, strings.Join(a.Binaries, ", "), manual)
			return nil
		}
		picked = tools.Installed{Agent: a, Binary: bin}
	case !env.IsTTY:
		fmt.Fprintln(env.Out, manual)
		return nil
	default:
		found := tools.Detect(env.Runner)
		if len(found) == 0 {
			fmt.Fprintln(env.Out, manual)
			return nil
		}
		fmt.Fprintln(env.Out, "\nTailor the rules to your situation now? An agent will interview you and rewrite them.")
		for i, a := range found {
			fmt.Fprintf(env.Out, "  %d) %s\n", i+1, a.Name)
		}
		fmt.Fprintln(env.Out, "  0) not now")
		n, err := askNumber(env, "Choose [0]: ", len(found))
		if err != nil {
			return err
		}
		if n == 0 {
			fmt.Fprintln(env.Out, manual)
			return nil
		}
		picked = found[n-1]
	}
	args := picked.Args(tools.HandoffPrompt)
	fmt.Fprintf(env.Out, "Starting %s: %s\n", picked.Name, tools.CommandLine(picked.Binary, args))
	if err := env.Runner.Interactive(dir, env.In, env.Out, env.Err, picked.Binary, args...); err != nil {
		return fmt.Errorf("%s exited: %w", picked.Name, err)
	}
	return nil
}

func printPlan(w io.Writer, p *scaffold.Plan) {
	for _, l := range p.Lines() {
		fmt.Fprintln(w, l)
	}
}

// confirm asks before writing. Without a terminal there is nobody to ask, so
// writing requires --yes.
func confirm(env Env, yes bool) error {
	if yes {
		return nil
	}
	if !env.IsTTY {
		return fmt.Errorf("%w: not a terminal; pass --yes to apply the changes or --dry-run to only show them", errUsage)
	}
	fmt.Fprint(env.Out, "Proceed? [Y/n] ")
	line, err := readLine(env.In)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
		return nil
	default:
		return errors.New("cancelled; nothing was changed")
	}
}

func askNumber(env Env, prompt string, max int) (int, error) {
	for {
		fmt.Fprint(env.Out, prompt)
		line, err := readLine(env.In)
		if err != nil {
			return 0, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return 0, nil
		}
		n, err := strconv.Atoi(line)
		if err == nil && n >= 0 && n <= max {
			return n, nil
		}
		fmt.Fprintf(env.Out, "Enter a number from 0 to %d.\n", max)
	}
}

// readLine reads one line; end of input counts as an empty answer. It reads
// byte by byte so that nothing after the line is consumed: the same input is
// later handed to the agent process.
func readLine(r io.Reader) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				return strings.TrimSuffix(sb.String(), "\r"), nil
			}
			sb.WriteByte(buf[0])
		}
		if errors.Is(err, io.EOF) {
			return sb.String(), nil
		}
		if err != nil {
			return "", err
		}
	}
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parse accepts flags before and after positional arguments, unlike
// flag.Parse, which stops at the first positional argument.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, fmt.Errorf("%w: %v", errUsage, err)
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		if args[0] == "--" {
			return append(pos, args[1:]...), nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}
