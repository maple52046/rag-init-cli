package content

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEmbeddedSnapshot(t *testing.T) {
	b, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"AGENTS.md",
		"CLAUDE.md",
		"docs/knowledge-governance.md",
		"knowledge/README.md",
		".agents/skills/create-knowledge-rules/SKILL.md",
		".agents/skills/git-commit/SKILL.md",
		".claude/skills/git-commit/SKILL.md",
		"knowledge/sources/README.md",
	} {
		if _, ok := b.Files[want]; !ok {
			t.Errorf("embedded snapshot lacks %s", want)
		}
	}
	if got, want := b.SkillNames(), []string{"create-knowledge-rules", "git-commit"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SkillNames() = %v, want %v", got, want)
	}
	ref, commit, ok := strings.Cut(EmbeddedRef(), " ")
	if !ok || ref == "" || len(commit) != 40 {
		t.Errorf("EmbeddedRef() = %q, want \"<ref> <40-hex commit>\"", EmbeddedRef())
	}
	if b.Version != ref || b.Source != "rag-init "+ref {
		t.Errorf("Version, Source = %q, %q; want %q, %q", b.Version, b.Source, ref, "rag-init "+ref)
	}
}

func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": "abc"}}); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "rag-init-main/", Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFromArchiveStripsTopDirectory(t *testing.T) {
	b, err := FromArchive(bytes.NewReader(archive(t, map[string]string{
		"rag-init-main/AGENTS.md":           "rules",
		"rag-init-main/.agents/skills/a/x":  "x",
		"rag-init-main/knowledge/README.md": "idx",
	})))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".agents/skills/a/x", "AGENTS.md", "knowledge/README.md"}
	if got := b.Paths(); !reflect.DeepEqual(got, want) {
		t.Errorf("Paths() = %v, want %v", got, want)
	}
	if string(b.Files["AGENTS.md"].Data) != "rules" {
		t.Errorf("AGENTS.md = %q", b.Files["AGENTS.md"].Data)
	}
}

func TestFromArchiveRejectsEscapingPaths(t *testing.T) {
	for _, name := range []string{"top/../../evil", "top/../x"} {
		_, err := FromArchive(bytes.NewReader(archive(t, map[string]string{name: "x", "top/AGENTS.md": "r"})))
		if err == nil || !strings.Contains(err.Error(), "unsafe path") {
			t.Errorf("FromArchive with %q: err = %v, want unsafe path", name, err)
		}
	}
}

func TestFromDir(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".git/config", "x")
	if _, err := FromDir(dir); err == nil {
		t.Fatal("FromDir without AGENTS.md succeeded")
	}
	write("AGENTS.md", "rules")
	write(".agents/skills/s/SKILL.md", "skill")
	b, err := FromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{".agents/skills/s/SKILL.md", "AGENTS.md"}; !reflect.DeepEqual(b.Paths(), want) {
		t.Errorf("Paths() = %v, want %v (.git must be skipped)", b.Paths(), want)
	}
	if b.Version != "local checkout" {
		t.Errorf("Version = %q", b.Version)
	}
}
