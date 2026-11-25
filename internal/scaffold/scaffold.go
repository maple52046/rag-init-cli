// Package scaffold turns a rag-init bundle into a list of file actions for a
// target directory and applies them. Planning never touches the disk beyond
// reading the target, so a plan can be shown before anything is written.
package scaffold

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/maple52046/rag-init-cli/internal/content"
)

// The always-on rules section in AGENTS.md is delimited by these markers; the
// create-knowledge-rules skill rewrites only the text between them.
const (
	markerStart = "<!-- knowledge-rules:start -->"
	markerEnd   = "<!-- knowledge-rules:end -->"
)

// Op is what an action does to its path.
type Op string

const (
	OpCreate    Op = "create"
	OpAppend    Op = "append"
	OpUpdate    Op = "update"
	OpReplace   Op = "replace"
	OpRemoveDir Op = "remove"
	OpSkip      Op = "skip"
)

// Action is one change to a path relative to the plan's directory.
type Action struct {
	Op   Op
	Path string
	// Group collects many files under one directory (a skill, the source
	// archive) into a single line when the plan is printed.
	Group string
	Note  string
	Data  []byte
	Mode  fs.FileMode
}

// Plan is the ordered list of actions for one directory.
type Plan struct {
	Dir     string
	Actions []Action
}

// Options controls what a plan includes.
type Options struct {
	// WithSources copies the methodology source archive (init only; new
	// always copies it, like the template).
	WithSources bool
	// UpdateSkills replaces skill folders that already exist (init only).
	UpdateSkills bool
	// Gemini adds AGENTS.md to Gemini CLI's context.fileName setting.
	Gemini bool
}

// excludedFromNew are rag-init's own documents, which describe rag-init
// rather than the user's repository.
var excludedFromNew = map[string]bool{
	"README.md":       true,
	"README.zh-TW.md": true,
	"CONTRIBUTING.md": true,
}

var starterFiles = []string{
	"docs/knowledge-governance.md",
	"knowledge/README.md",
	"knowledge/taxonomy.md",
	"knowledge/glossary.md",
}

// PlanNew plans a new knowledge-base repository in dir, which must not exist
// or must be empty.
func PlanNew(b *content.Bundle, dir string, opts Options) (*Plan, error) {
	if err := requireEmptyOrMissing(dir); err != nil {
		return nil, err
	}
	p := &Plan{Dir: dir}
	for _, rel := range b.Paths() {
		if excludedFromNew[rel] {
			continue
		}
		f := b.Files[rel]
		p.add(Action{Op: OpCreate, Path: rel, Group: groupOf(rel), Data: f.Data, Mode: f.Mode})
	}
	p.add(Action{Op: OpCreate, Path: "README.md", Data: newReadme(filepath.Base(filepath.Clean(dir)), b.Version), Mode: 0o644})
	if opts.Gemini {
		if err := p.planGemini(); err != nil {
			return nil, err
		}
	}
	p.sort()
	return p, nil
}

// PlanInit plans adding rag-init to an existing project in dir. Existing
// files are never overwritten, except skill folders with UpdateSkills.
func PlanInit(b *content.Bundle, dir string, opts Options) (*Plan, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	p := &Plan{Dir: dir}

	for _, name := range b.SkillNames() {
		wrote := p.planTree(b, ".agents/skills/"+name, opts.UpdateSkills)
		if len(b.Under(".claude/skills/"+name)) == 0 {
			continue
		}
		// The Claude Code wrapper points to .agents/skills/<name>; next to a
		// skill the user kept, it would describe the wrong skill.
		if !wrote {
			p.add(Action{Op: OpSkip, Path: ".claude/skills/" + name + "/", Note: "kept because .agents/skills/" + name + " was kept"})
			continue
		}
		p.planTree(b, ".claude/skills/"+name, opts.UpdateSkills)
	}
	if err := p.planAgentsMD(b); err != nil {
		return nil, err
	}
	if err := p.planClaudeMD(); err != nil {
		return nil, err
	}
	for _, rel := range starterFiles {
		f, ok := b.Files[rel]
		if !ok {
			continue
		}
		p.createIfMissing(rel, "", f.Data, f.Mode)
	}
	if opts.WithSources {
		for _, rel := range b.Under("knowledge/sources") {
			f := b.Files[rel]
			p.createIfMissing(rel, "knowledge/sources/", f.Data, f.Mode)
		}
		if f, ok := b.Files["THIRD_PARTY_NOTICES.md"]; ok {
			if p.exists("THIRD_PARTY_NOTICES.md") {
				p.createIfMissing("THIRD_PARTY_NOTICES.rag-init.md", "", f.Data, f.Mode)
			} else {
				p.add(Action{Op: OpCreate, Path: "THIRD_PARTY_NOTICES.md", Data: f.Data, Mode: f.Mode})
			}
		}
	} else {
		p.createIfMissing("knowledge/sources/README.md", "", sourcesStub, 0o644)
	}
	if opts.Gemini {
		if err := p.planGemini(); err != nil {
			return nil, err
		}
	}
	p.sort()
	return p, nil
}

// Apply performs the plan's actions in order.
func (p *Plan) Apply() error {
	for _, a := range p.Actions {
		target := filepath.Join(p.Dir, filepath.FromSlash(a.Path))
		var err error
		switch a.Op {
		case OpSkip:
			continue
		case OpRemoveDir:
			err = os.RemoveAll(target)
		case OpCreate:
			err = writeFile(target, a.Data, a.Mode, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		case OpReplace, OpUpdate:
			err = writeFile(target, a.Data, a.Mode, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
		case OpAppend:
			err = writeFile(target, a.Data, a.Mode, os.O_WRONLY|os.O_CREATE|os.O_APPEND)
		default:
			err = fmt.Errorf("unknown action %q", a.Op)
		}
		if err != nil {
			return fmt.Errorf("%s %s: %w", a.Op, a.Path, err)
		}
	}
	return nil
}

// Changes reports whether the plan writes anything.
func (p *Plan) Changes() bool {
	for _, a := range p.Actions {
		if a.Op != OpSkip {
			return true
		}
	}
	return false
}

// Lines renders the plan for people: one line per action, with grouped files
// folded into one line per group and operation.
func (p *Plan) Lines() []string {
	type key struct {
		op    Op
		group string
	}
	counts := map[key]int{}
	var order []key
	var lines []string
	for _, a := range p.Actions {
		if a.Group != "" && a.Op != OpRemoveDir {
			k := key{a.Op, a.Group}
			if counts[k] == 0 {
				order = append(order, k)
				lines = append(lines, "")
			}
			counts[k]++
			continue
		}
		line := fmt.Sprintf("%-7s %s", a.Op, a.Path)
		if a.Note != "" {
			line += "  (" + a.Note + ")"
		}
		lines = append(lines, line)
	}
	// Fill the placeholders reserved for groups, in first-seen order.
	gi := 0
	for i, l := range lines {
		if l != "" {
			continue
		}
		k := order[gi]
		gi++
		noun := "files"
		if counts[k] == 1 {
			noun = "file"
		}
		lines[i] = fmt.Sprintf("%-7s %s (%d %s)", k.op, k.group, counts[k], noun)
	}
	return lines
}

func (p *Plan) add(a Action) {
	if a.Mode == 0 {
		a.Mode = 0o644
	}
	p.Actions = append(p.Actions, a)
}

// sort orders actions by path, keeping a directory removal before the files
// written into it.
func (p *Plan) sort() {
	sort.SliceStable(p.Actions, func(i, j int) bool {
		ai, aj := p.Actions[i], p.Actions[j]
		if ai.Path != aj.Path {
			return ai.Path < aj.Path
		}
		return ai.Op == OpRemoveDir && aj.Op != OpRemoveDir
	})
}

func (p *Plan) exists(rel string) bool {
	_, err := os.Lstat(filepath.Join(p.Dir, filepath.FromSlash(rel)))
	return err == nil
}

func (p *Plan) read(rel string) ([]byte, bool, error) {
	data, err := os.ReadFile(filepath.Join(p.Dir, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (p *Plan) createIfMissing(rel, group string, data []byte, mode fs.FileMode) {
	if p.exists(rel) {
		p.add(Action{Op: OpSkip, Path: rel, Group: group, Note: "already exists"})
		return
	}
	p.add(Action{Op: OpCreate, Path: rel, Group: group, Data: data, Mode: mode})
}

// planTree copies one skill folder and reports whether it writes it. An
// existing folder is left alone unless replace is set; then it is removed
// first so no stale files remain.
func (p *Plan) planTree(b *content.Bundle, dir string, replace bool) bool {
	group := dir + "/"
	files := b.Under(dir)
	if p.exists(dir) {
		if !replace {
			p.add(Action{Op: OpSkip, Path: group, Note: "already exists; use --update-skills to replace it"})
			return false
		}
		p.add(Action{Op: OpRemoveDir, Path: dir, Note: "replaced with the rag-init version"})
		for _, rel := range files {
			f := b.Files[rel]
			p.add(Action{Op: OpReplace, Path: rel, Group: group, Data: f.Data, Mode: f.Mode})
		}
		return true
	}
	for _, rel := range files {
		f := b.Files[rel]
		p.add(Action{Op: OpCreate, Path: rel, Group: group, Data: f.Data, Mode: f.Mode})
	}
	return true
}

func (p *Plan) planAgentsMD(b *content.Bundle) error {
	src, ok := b.Files["AGENTS.md"]
	if !ok {
		return errors.New("rag-init content has no AGENTS.md")
	}
	section, err := rulesSection(src.Data)
	if err != nil {
		return err
	}
	cur, found, err := p.read("AGENTS.md")
	if err != nil {
		return err
	}
	switch {
	case !found:
		p.add(Action{Op: OpCreate, Path: "AGENTS.md", Data: src.Data})
	case bytes.Contains(cur, []byte(markerStart)):
		p.add(Action{Op: OpSkip, Path: "AGENTS.md", Note: "already has a knowledge-rules section"})
	default:
		p.add(Action{Op: OpAppend, Path: "AGENTS.md", Note: "adds the knowledge-rules section at the end", Data: appendBlock(cur, section)})
	}
	return nil
}

func (p *Plan) planClaudeMD() error {
	cur, found, err := p.read("CLAUDE.md")
	if err != nil {
		return err
	}
	switch {
	case !found:
		p.add(Action{Op: OpCreate, Path: "CLAUDE.md", Data: []byte("@AGENTS.md\n")})
	case hasLine(cur, "@AGENTS.md"):
		p.add(Action{Op: OpSkip, Path: "CLAUDE.md", Note: "already imports AGENTS.md"})
	default:
		p.add(Action{Op: OpAppend, Path: "CLAUDE.md", Note: "adds @AGENTS.md so Claude Code reads the rules", Data: appendBlock(cur, []byte("@AGENTS.md\n"))})
	}
	return nil
}

// planGemini makes Gemini CLI read AGENTS.md by listing it in
// context.fileName of .gemini/settings.json, keeping GEMINI.md and every other
// setting.
func (p *Plan) planGemini() error {
	const rel = ".gemini/settings.json"
	cur, found, err := p.read(rel)
	if err != nil {
		return err
	}
	settings := map[string]any{}
	if found {
		if err := json.Unmarshal(cur, &settings); err != nil {
			p.add(Action{Op: OpSkip, Path: rel, Note: "not plain JSON; add \"AGENTS.md\" to context.fileName by hand"})
			return nil
		}
	}
	ctx, _ := settings["context"].(map[string]any)
	if ctx == nil {
		ctx = map[string]any{}
	}
	var names []string
	switch v := ctx["fileName"].(type) {
	case string:
		names = []string{v}
	case []any:
		for _, n := range v {
			if s, ok := n.(string); ok {
				names = append(names, s)
			}
		}
	}
	if len(names) == 0 {
		names = []string{"GEMINI.md"}
	}
	for _, n := range names {
		if n == "AGENTS.md" {
			p.add(Action{Op: OpSkip, Path: rel, Note: "already lists AGENTS.md"})
			return nil
		}
	}
	ctx["fileName"] = append([]string{"AGENTS.md"}, names...)
	settings["context"] = ctx
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if found {
		p.add(Action{Op: OpUpdate, Path: rel, Note: "adds AGENTS.md to context.fileName", Data: out})
	} else {
		p.add(Action{Op: OpCreate, Path: rel, Data: out})
	}
	return nil
}

// rulesSection returns the knowledge-rules section of an AGENTS.md, markers
// included.
func rulesSection(agents []byte) ([]byte, error) {
	start := bytes.Index(agents, []byte(markerStart))
	end := bytes.Index(agents, []byte(markerEnd))
	if start < 0 || end < start {
		return nil, errors.New("rag-init AGENTS.md has no knowledge-rules section")
	}
	return append(bytes.Clone(agents[start:end+len(markerEnd)]), '\n'), nil
}

// appendBlock returns the bytes to append to cur so that block starts after
// one blank line.
func appendBlock(cur, block []byte) []byte {
	var sep []byte
	switch {
	case len(cur) == 0:
	case bytes.HasSuffix(cur, []byte("\n\n")):
	case bytes.HasSuffix(cur, []byte("\n")):
		sep = []byte("\n")
	default:
		sep = []byte("\n\n")
	}
	return append(sep, block...)
}

func hasLine(data []byte, want string) bool {
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == want {
			return true
		}
	}
	return false
}

// groupOf folds the files of a skill folder or the source archive into one
// line of the printed plan.
func groupOf(rel string) string {
	parts := strings.Split(rel, "/")
	switch {
	case len(parts) >= 4 && (parts[0] == ".agents" || parts[0] == ".claude") && parts[1] == "skills":
		return path.Join(parts[:3]...) + "/"
	case len(parts) >= 3 && parts[0] == "knowledge" && parts[1] == "sources":
		return "knowledge/sources/"
	}
	return ""
}

func requireEmptyOrMissing(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s already exists and is not empty; use `rag-init init` to add rag-init to an existing project", dir)
	}
	return nil
}

func writeFile(target string, data []byte, mode fs.FileMode, flag int) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, flag, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

var sourcesStub = []byte(`# Source archive

External originals and summaries of them, kept apart from this knowledge base's own documents (rule R-DIST-6
in [the knowledge rules](../../docs/knowledge-governance.md)). Keep originals unchanged, record their URL,
version or date, and license here, and leave this folder out of routine searches.

rag-init ships a methodology archive — Diátaxis, ADR, Zettelkasten, ISO 30401, Google developer style, The Good
Docs Project, and the Agent Skills standard — that you can add with ` + "`rag-init init --with-sources`" + `.
`)

func newReadme(name, source string) []byte {
	return []byte(fmt.Sprintf(`# %s

A knowledge base that people and agents read and write, created with
[rag-init](https://github.com/maple52046/rag-init) %s.

- Knowledge lives in `+"`knowledge/`"+`; start from [`+"`knowledge/README.md`"+`](knowledge/README.md).
- The rules for reading and writing knowledge are in
  [`+"`docs/knowledge-governance.md`"+`](docs/knowledge-governance.md), with an always-on summary in
  [`+"`AGENTS.md`"+`](AGENTS.md).
- To fit the rules to your situation, ask your agent to run the `+"`create-knowledge-rules`"+` skill
  (`+"`/create-knowledge-rules`"+`, or `+"`$create-knowledge-rules`"+` in Codex).
- `+"`LICENSE`"+` covers the files that came from rag-init; choose a license for your own content. Keep
  `+"`THIRD_PARTY_NOTICES.md`"+` for as long as you keep `+"`knowledge/sources/`"+`.
`, name, source))
}
