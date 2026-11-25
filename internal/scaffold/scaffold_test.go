package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/maple52046/rag-init-cli/internal/content"
)

const agentsMD = "# Agent instructions\n\nIntro.\n\n" + markerStart + "\n## Knowledge base\n- rule\n" + markerEnd + "\n"

func bundle() *content.Bundle {
	files := map[string]string{
		"AGENTS.md":                               agentsMD,
		"CLAUDE.md":                               "@AGENTS.md\n",
		"README.md":                               "about rag-init",
		"README.zh-TW.md":                         "關於 rag-init",
		"CONTRIBUTING.md":                         "maintain rag-init",
		"LICENSE":                                 "MIT",
		"THIRD_PARTY_NOTICES.md":                  "notices",
		"docs/knowledge-governance.md":            "rules doc",
		"knowledge/README.md":                     "index",
		"knowledge/taxonomy.md":                   "taxonomy",
		"knowledge/glossary.md":                   "glossary",
		"knowledge/sources/README.md":             "sources manifest",
		"knowledge/sources/ADR/README.md":         "adr",
		".agents/skills/git-commit/SKILL.md":      "git-commit skill",
		".agents/skills/git-commit/references/cc": "spec",
		".claude/skills/git-commit/SKILL.md":      "git-commit wrapper",
	}
	b := &content.Bundle{Source: "rag-init test", Version: "test", Files: map[string]content.File{}}
	for p, d := range files {
		b.Files[p] = content.File{Data: []byte(d), Mode: 0o644}
	}
	return b
}

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func exists(dir, rel string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil
}

func ops(p *Plan) map[string]Op {
	m := map[string]Op{}
	for _, a := range p.Actions {
		m[a.Path] = a.Op
	}
	return m
}

func TestPlanNew(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kb")
	p, err := PlanNew(bundle(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md", "LICENSE", "THIRD_PARTY_NOTICES.md", "knowledge/sources/ADR/README.md", ".claude/skills/git-commit/SKILL.md"} {
		if !exists(dir, rel) {
			t.Errorf("%s was not created", rel)
		}
	}
	for _, rel := range []string{"README.zh-TW.md", "CONTRIBUTING.md"} {
		if exists(dir, rel) {
			t.Errorf("%s must not be copied into a new repository", rel)
		}
	}
	readme := read(t, dir, "README.md")
	if !strings.HasPrefix(readme, "# kb\n") || !strings.Contains(readme, "rag-init) test.") {
		t.Errorf("generated README.md:\n%s", readme)
	}
}

func TestPlanNewRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "x", "x")
	if _, err := PlanNew(bundle(), dir, Options{}); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("err = %v, want not empty", err)
	}
}

func TestPlanInitIntoEmptyProject(t *testing.T) {
	dir := t.TempDir()
	p, err := PlanInit(bundle(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "AGENTS.md"); got != agentsMD {
		t.Errorf("AGENTS.md = %q, want the rag-init file", got)
	}
	if got := read(t, dir, "CLAUDE.md"); got != "@AGENTS.md\n" {
		t.Errorf("CLAUDE.md = %q", got)
	}
	if got := read(t, dir, "knowledge/sources/README.md"); !strings.Contains(got, "--with-sources") {
		t.Errorf("without --with-sources, knowledge/sources/README.md should be the stub, got %q", got)
	}
	for _, rel := range []string{"knowledge/sources/ADR/README.md", "THIRD_PARTY_NOTICES.md", "LICENSE", "README.md", "CONTRIBUTING.md"} {
		if exists(dir, rel) {
			t.Errorf("init must not create %s", rel)
		}
	}
	for _, rel := range []string{".agents/skills/git-commit/references/cc", ".claude/skills/git-commit/SKILL.md", "docs/knowledge-governance.md", "knowledge/glossary.md"} {
		if !exists(dir, rel) {
			t.Errorf("%s was not created", rel)
		}
	}
	// A second run changes nothing.
	p, err = PlanInit(bundle(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Changes() {
		t.Errorf("second init plans changes: %v", p.Lines())
	}
}

func TestPlanInitMergesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "AGENTS.md", "# Mine\n\nUse pnpm.")
	write(t, dir, "CLAUDE.md", "Be terse.\n")
	p, err := PlanInit(bundle(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	got := read(t, dir, "AGENTS.md")
	want := "# Mine\n\nUse pnpm.\n\n" + markerStart + "\n## Knowledge base\n- rule\n" + markerEnd + "\n"
	if got != want {
		t.Errorf("AGENTS.md =\n%q\nwant\n%q", got, want)
	}
	if got := read(t, dir, "CLAUDE.md"); got != "Be terse.\n\n@AGENTS.md\n" {
		t.Errorf("CLAUDE.md = %q", got)
	}
}

func TestPlanInitKeepsExistingRulesAndImport(t *testing.T) {
	dir := t.TempDir()
	tailored := "# Mine\n" + markerStart + "\ncustom\n" + markerEnd + "\n"
	write(t, dir, "AGENTS.md", tailored)
	write(t, dir, "CLAUDE.md", "  @AGENTS.md  \n")
	p, err := PlanInit(bundle(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	m := ops(p)
	if m["AGENTS.md"] != OpSkip || m["CLAUDE.md"] != OpSkip {
		t.Errorf("ops = %v, want AGENTS.md and CLAUDE.md skipped", m)
	}
}

func TestPlanInitSkills(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".agents/skills/git-commit/SKILL.md", "my own skill")
	write(t, dir, ".agents/skills/git-commit/stale.txt", "old")

	p, err := PlanInit(bundle(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, ".agents/skills/git-commit/SKILL.md"); got != "my own skill" {
		t.Errorf("an existing skill was overwritten without --update-skills: %q", got)
	}
	if exists(dir, ".claude/skills/git-commit/SKILL.md") {
		t.Error("the Claude wrapper was created next to a skill the user kept")
	}

	p, err = PlanInit(bundle(), dir, Options{UpdateSkills: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, ".agents/skills/git-commit/SKILL.md"); got != "git-commit skill" {
		t.Errorf("--update-skills did not replace SKILL.md: %q", got)
	}
	if exists(dir, ".agents/skills/git-commit/stale.txt") {
		t.Error("--update-skills left a stale file behind")
	}
	if got := read(t, dir, ".claude/skills/git-commit/SKILL.md"); got != "git-commit wrapper" {
		t.Errorf("Claude wrapper = %q", got)
	}
}

func TestPlanInitWithSources(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "THIRD_PARTY_NOTICES.md", "their notices")
	p, err := PlanInit(bundle(), dir, Options{WithSources: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "knowledge/sources/README.md"); got != "sources manifest" {
		t.Errorf("knowledge/sources/README.md = %q, want the rag-init manifest", got)
	}
	if !exists(dir, "knowledge/sources/ADR/README.md") {
		t.Error("sources were not copied")
	}
	if got := read(t, dir, "THIRD_PARTY_NOTICES.md"); got != "their notices" {
		t.Errorf("existing THIRD_PARTY_NOTICES.md was changed: %q", got)
	}
	if got := read(t, dir, "THIRD_PARTY_NOTICES.rag-init.md"); got != "notices" {
		t.Errorf("THIRD_PARTY_NOTICES.rag-init.md = %q", got)
	}
}

func geminiNames(t *testing.T, dir string) (map[string]any, []any) {
	t.Helper()
	var s map[string]any
	if err := json.Unmarshal([]byte(read(t, dir, ".gemini/settings.json")), &s); err != nil {
		t.Fatal(err)
	}
	ctx, _ := s["context"].(map[string]any)
	names, _ := ctx["fileName"].([]any)
	return s, names
}

func TestPlanGemini(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		dir := t.TempDir()
		p, err := PlanInit(bundle(), dir, Options{Gemini: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Apply(); err != nil {
			t.Fatal(err)
		}
		if _, names := geminiNames(t, dir); !reflect.DeepEqual(names, []any{"AGENTS.md", "GEMINI.md"}) {
			t.Errorf("fileName = %v", names)
		}
	})
	t.Run("string fileName and other keys", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, ".gemini/settings.json", `{"theme":"dark","context":{"fileName":"CONTEXT.md","x":1}}`)
		p, err := PlanInit(bundle(), dir, Options{Gemini: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Apply(); err != nil {
			t.Fatal(err)
		}
		s, names := geminiNames(t, dir)
		if !reflect.DeepEqual(names, []any{"AGENTS.md", "CONTEXT.md"}) {
			t.Errorf("fileName = %v", names)
		}
		if s["theme"] != "dark" || s["context"].(map[string]any)["x"] != float64(1) {
			t.Errorf("other settings were lost: %v", s)
		}
	})
	t.Run("already listed or not JSON", func(t *testing.T) {
		for _, body := range []string{`{"context":{"fileName":["GEMINI.md","AGENTS.md"]}}`, "// comment\n{}"} {
			dir := t.TempDir()
			write(t, dir, ".gemini/settings.json", body)
			p, err := PlanInit(bundle(), dir, Options{Gemini: true})
			if err != nil {
				t.Fatal(err)
			}
			if op := ops(p)[".gemini/settings.json"]; op != OpSkip {
				t.Errorf("settings %q: op = %s, want skip", body, op)
			}
		}
	})
}

func TestLinesGroupsFiles(t *testing.T) {
	p, err := PlanNew(bundle(), filepath.Join(t.TempDir(), "kb"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Join(p.Lines(), "\n")
	for _, want := range []string{"create  .agents/skills/git-commit/ (2 files)", "create  knowledge/sources/ (2 files)", "create  AGENTS.md"} {
		if !strings.Contains(lines, want) {
			t.Errorf("plan lines lack %q:\n%s", want, lines)
		}
	}
}

func TestApplyNeverOverwritesOnCreate(t *testing.T) {
	dir := t.TempDir()
	p, err := PlanInit(bundle(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// A file appears between planning and applying.
	write(t, dir, "docs/knowledge-governance.md", "written meanwhile")
	if err := p.Apply(); err == nil {
		t.Fatal("Apply overwrote a file created after planning")
	}
	if got := read(t, dir, "docs/knowledge-governance.md"); got != "written meanwhile" {
		t.Errorf("file = %q", got)
	}
}
