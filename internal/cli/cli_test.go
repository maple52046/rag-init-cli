package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maple52046/rag-init-cli/internal/content"
	"github.com/maple52046/rag-init-cli/internal/tools"
)

type call struct {
	dir  string
	name string
	args []string
}

type fakeRunner struct {
	installed   map[string]bool
	calls       []call
	interactive []call
	failGit     string // git subcommand that fails
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	if f.installed[name] {
		return "/usr/bin/" + name, nil
	}
	return "", errors.New("not found")
}

func (f *fakeRunner) Output(dir, name string, args ...string) (string, error) {
	f.calls = append(f.calls, call{dir, name, args})
	if name == "git" && len(args) > 0 && args[0] == f.failGit {
		return "Author identity unknown", errors.New("exit status 128")
	}
	return "", nil
}

func (f *fakeRunner) Interactive(dir string, _ io.Reader, _, _ io.Writer, name string, args ...string) error {
	f.interactive = append(f.interactive, call{dir, name, args})
	return nil
}

func run2(t *testing.T, r *fakeRunner, tty bool, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(args, Env{
		In:      strings.NewReader(stdin),
		Out:     &out,
		Err:     &errOut,
		IsTTY:   tty,
		Runner:  r,
		Load:    func(ref, from string) (*content.Bundle, error) { return content.Load(ref, from) },
		Version: "v9.9.9",
	})
	return code, out.String(), errOut.String()
}

func TestVersionAndHelp(t *testing.T) {
	code, out, _ := run2(t, &fakeRunner{}, false, "", "version")
	if code != 0 || !strings.Contains(out, "rag-init v9.9.9") || !strings.Contains(out, "embedded rag-init content: ") {
		t.Errorf("version: code %d, out %q", code, out)
	}
	code, out, _ = run2(t, &fakeRunner{}, false, "")
	if code != 0 || !strings.Contains(out, "rag-init new <dir>") {
		t.Errorf("no args: code %d, out %q", code, out)
	}
	if code, _, errOut := run2(t, &fakeRunner{}, false, "", "bogus"); code != 2 || !strings.Contains(errOut, "unknown command") {
		t.Errorf("bogus: code %d, err %q", code, errOut)
	}
}

func TestNewDryRunWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kb")
	r := &fakeRunner{installed: map[string]bool{"git": true}}
	code, out, errOut := run2(t, r, false, "", "new", dir, "--dry-run")
	if code != 0 {
		t.Fatalf("code %d: %s", code, errOut)
	}
	if !strings.Contains(out, "create  AGENTS.md") || !strings.Contains(out, "then    git init") {
		t.Errorf("plan output:\n%s", out)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("--dry-run created %s", dir)
	}
	if len(r.calls) != 0 {
		t.Errorf("--dry-run ran %v", r.calls)
	}
}

func TestNewWithoutTerminalNeedsYes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kb")
	code, _, errOut := run2(t, &fakeRunner{}, false, "", "new", dir)
	if code != 2 || !strings.Contains(errOut, "--yes") {
		t.Errorf("code %d, err %q", code, errOut)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Error("files were written without confirmation")
	}
}

func TestNewCommitsAndCreatesGitHubRepo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kb")
	r := &fakeRunner{installed: map[string]bool{"git": true, "gh": true}}
	code, out, errOut := run2(t, r, false, "", "new", dir, "--github", "me/kb", "--yes", "--agent", "none")
	if code != 0 {
		t.Fatalf("code %d: %s", code, errOut)
	}
	var got []string
	for _, c := range r.calls {
		got = append(got, c.name+" "+strings.Join(c.args, " "))
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"git init -q -b main", "git add -A", "git commit -q -m chore: start knowledge base from rag-init", "gh repo create me/kb --private --source . --remote origin --push"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in calls:\n%s", want, joined)
		}
	}
	if !strings.Contains(out, "Created the GitHub repository me/kb") {
		t.Errorf("out:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Error(err)
	}
}

func TestNewSkipsGitHubWhenCommitFails(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kb")
	r := &fakeRunner{installed: map[string]bool{"git": true, "gh": true}, failGit: "commit"}
	code, _, errOut := run2(t, r, false, "", "new", dir, "--github", "me/kb", "--yes", "--agent", "none")
	if code != 0 {
		t.Fatalf("code %d: %s", code, errOut)
	}
	if !strings.Contains(errOut, "commit them yourself") {
		t.Errorf("err:\n%s", errOut)
	}
	for _, c := range r.calls {
		if c.name == "gh" {
			t.Error("gh ran although the first commit failed")
		}
	}
}

func TestInitHandsOverToChosenAgent(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRunner{installed: map[string]bool{"agy": true}}
	code, out, errOut := run2(t, r, false, "", "init", dir, "--yes", "--agent", "antigravity")
	if code != 0 {
		t.Fatalf("code %d: %s", code, errOut)
	}
	if len(r.interactive) != 1 {
		t.Fatalf("interactive calls = %v", r.interactive)
	}
	c := r.interactive[0]
	if c.name != "agy" || c.dir != dir || len(c.args) != 2 || c.args[0] != "-i" || c.args[1] != tools.HandoffPrompt {
		t.Errorf("agent call = %+v", c)
	}
	if !strings.Contains(out, "Starting Antigravity CLI") {
		t.Errorf("out:\n%s", out)
	}
}

func TestInitOffersInstalledAgentsOnTerminal(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRunner{installed: map[string]bool{"claude": true, "agent": true}}
	// Confirm the plan, then pick the second agent offered (Cursor CLI).
	code, out, errOut := run2(t, r, true, "y\n2\n", "init", dir)
	if code != 0 {
		t.Fatalf("code %d: %s", code, errOut)
	}
	if !strings.Contains(out, "1) Claude Code") || !strings.Contains(out, "2) Cursor CLI") {
		t.Errorf("menu:\n%s", out)
	}
	if len(r.interactive) != 1 || r.interactive[0].name != "agent" || r.interactive[0].args[0] != tools.HandoffPrompt {
		t.Errorf("interactive = %+v", r.interactive)
	}
}

func TestInitDeclinedChangesNothing(t *testing.T) {
	dir := t.TempDir()
	code, _, errOut := run2(t, &fakeRunner{}, true, "n\n", "init", dir)
	if code != 1 || !strings.Contains(errOut, "cancelled") {
		t.Errorf("code %d, err %q", code, errOut)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("declined init wrote %d entries", len(entries))
	}
}

func TestFlagErrors(t *testing.T) {
	for _, args := range [][]string{
		{"new"},
		{"new", "a", "b"},
		{"new", "kb", "--agent", "vim"},
		{"new", "kb", "--ref", "main", "--from", "."},
		{"new", "kb", "--github", "me/kb", "--no-git"},
		{"init", "a", "b"},
		{"init", "--nope"},
	} {
		if code, _, _ := run2(t, &fakeRunner{}, false, "", args...); code != 2 {
			t.Errorf("%v: code %d, want 2", args, code)
		}
	}
}

func TestFlagsAfterPositional(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kb")
	code, out, errOut := run2(t, &fakeRunner{}, false, "", "new", dir, "--no-git", "--dry-run")
	if code != 0 || strings.Contains(out, "git init") {
		t.Errorf("code %d, out %q, err %q", code, out, errOut)
	}
}
