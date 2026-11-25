// Package tools runs the external programs the CLI relies on: git, the
// GitHub CLI, and the coding agents it hands over to.
package tools

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Runner runs external commands. Tests replace it to avoid real processes.
type Runner interface {
	LookPath(name string) (string, error)
	// Output runs a command and returns its combined output.
	Output(dir, name string, args ...string) (string, error)
	// Interactive runs a command attached to the given streams.
	Interactive(dir string, in io.Reader, out, errOut io.Writer, name string, args ...string) error
}

// System runs real processes.
type System struct{}

func (System) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (System) Output(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

func (System) Interactive(dir string, in io.Reader, out, errOut io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = errOut
	return cmd.Run()
}

// GitInit creates a repository on branch main in dir and commits every file.
// A missing git identity makes the commit fail; the repository is still
// created, so the caller can report and let the user commit.
func GitInit(r Runner, dir, message string) error {
	if _, err := r.LookPath("git"); err != nil {
		return fmt.Errorf("git is not installed: %w", err)
	}
	steps := [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{"commit", "-q", "-m", message},
	}
	for _, args := range steps {
		if out, err := r.Output(dir, "git", args...); err != nil {
			return fmt.Errorf("git %s: %v: %s", args[0], err, out)
		}
	}
	return nil
}

// CreateGitHubRepo creates repo (owner/name or name) on GitHub from dir with
// the GitHub CLI and pushes the current branch.
func CreateGitHubRepo(r Runner, dir, repo string, private bool) error {
	if _, err := r.LookPath("gh"); err != nil {
		return fmt.Errorf("the GitHub CLI (gh) is not installed: %w", err)
	}
	visibility := "--private"
	if !private {
		visibility = "--public"
	}
	if out, err := r.Output(dir, "gh", "repo", "create", repo, visibility, "--source", ".", "--remote", "origin", "--push"); err != nil {
		return fmt.Errorf("gh repo create: %v: %s", err, out)
	}
	return nil
}

// HandoffPrompt is the first message given to the agent after setup.
const HandoffPrompt = "Use the create-knowledge-rules skill to tailor this repository's knowledge rules to my situation."

// Agent is a coding-agent CLI the setup can hand over to.
type Agent struct {
	Key  string
	Name string
	// Binaries are tried in order; the first one found is used.
	Binaries []string
	// Args builds the arguments that start an interactive session with an
	// initial prompt.
	Args func(prompt string) []string
}

// Agents lists the supported agent CLIs, in the order they are offered.
var Agents = []Agent{
	{Key: "claude", Name: "Claude Code", Binaries: []string{"claude"}, Args: func(p string) []string { return []string{p} }},
	{Key: "codex", Name: "OpenAI Codex", Binaries: []string{"codex"}, Args: func(p string) []string { return []string{p} }},
	{Key: "cursor", Name: "Cursor CLI", Binaries: []string{"cursor-agent", "agent"}, Args: func(p string) []string { return []string{p} }},
	{Key: "antigravity", Name: "Antigravity CLI", Binaries: []string{"agy"}, Args: func(p string) []string { return []string{"-i", p} }},
	{Key: "gemini", Name: "Gemini CLI", Binaries: []string{"gemini"}, Args: func(p string) []string { return []string{"-i", p} }},
}

// AgentByKey finds an agent by its key.
func AgentByKey(key string) (Agent, bool) {
	for _, a := range Agents {
		if a.Key == key {
			return a, true
		}
	}
	return Agent{}, false
}

// AgentKeys returns the keys accepted by --agent.
func AgentKeys() []string {
	keys := make([]string, 0, len(Agents))
	for _, a := range Agents {
		keys = append(keys, a.Key)
	}
	return keys
}

// Installed is an agent together with the binary found for it.
type Installed struct {
	Agent
	Binary string
}

// Find returns the binary for a, if any is on PATH.
func Find(r Runner, a Agent) (string, bool) {
	for _, b := range a.Binaries {
		if _, err := r.LookPath(b); err == nil {
			return b, true
		}
	}
	return "", false
}

// Detect returns the supported agents that are installed.
func Detect(r Runner) []Installed {
	var out []Installed
	for _, a := range Agents {
		if b, ok := Find(r, a); ok {
			out = append(out, Installed{Agent: a, Binary: b})
		}
	}
	return out
}

// CommandLine renders the command that starts agent a in a shell-friendly
// form, for messages.
func CommandLine(binary string, args []string) string {
	parts := []string{binary}
	for _, a := range args {
		if strings.ContainsAny(a, " '\"$") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}
