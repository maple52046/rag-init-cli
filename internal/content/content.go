// Package content loads the rag-init files that the CLI lays out: the
// snapshot embedded at build time, a GitHub archive of another ref, or a
// local checkout.
package content

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Repository is the GitHub repository the content comes from.
const Repository = "maple52046/rag-init"

//go:embed snapshot/rag-init.tar.gz
var snapshotArchive []byte

//go:embed snapshot/ref.txt
var snapshotRef string

// File is one regular file of a bundle.
type File struct {
	Data []byte
	Mode fs.FileMode
}

// Bundle is the set of rag-init files, keyed by slash-separated paths
// relative to the rag-init repository root.
type Bundle struct {
	// Source describes where the files came from, for messages and the
	// initial commit, for example "rag-init v0.1.0".
	Source string
	// Version is the rag-init ref, or "local checkout" for FromDir.
	Version string
	Files   map[string]File
}

// EmbeddedRef returns the rag-init ref and commit recorded for the embedded
// snapshot, as "ref commit".
func EmbeddedRef() string {
	return strings.TrimSpace(snapshotRef)
}

// Embedded returns the snapshot compiled into the binary.
func Embedded() (*Bundle, error) {
	ref, _, _ := strings.Cut(EmbeddedRef(), " ")
	b, err := FromArchive(bytes.NewReader(snapshotArchive))
	if err != nil {
		return nil, fmt.Errorf("read embedded rag-init snapshot: %w", err)
	}
	b.Source = "rag-init " + ref
	b.Version = ref
	return b, nil
}

// ArchiveURL is the GitHub archive URL for a ref of rag-init.
func ArchiveURL(ref string) string {
	return "https://codeload.github.com/" + Repository + "/tar.gz/" + ref
}

// Download fetches the GitHub archive of ref (a tag, branch, or commit).
func Download(ref string) (*Bundle, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(ArchiveURL(ref))
	if err != nil {
		return nil, fmt.Errorf("download rag-init %s: %w", ref, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download rag-init %s: %s (does the ref exist?)", ref, resp.Status)
	}
	b, err := FromArchive(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rag-init %s archive: %w", ref, err)
	}
	b.Source = "rag-init " + ref
	b.Version = ref
	return b, nil
}

// FromArchive reads a gzip-compressed tar archive in the layout GitHub
// produces: every entry sits under one top-level directory, which is
// stripped.
func FromArchive(r io.Reader) (*Bundle, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	b := &Bundle{Files: map[string]File{}}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		_, rel, ok := strings.Cut(hdr.Name, "/")
		if !ok || rel == "" {
			continue
		}
		rel, err = cleanRel(rel)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		b.Files[rel] = File{Data: data, Mode: fs.FileMode(hdr.Mode).Perm()}
	}
	if len(b.Files) == 0 {
		return nil, errors.New("archive contains no files")
	}
	return b, nil
}

// FromDir reads a local rag-init checkout, skipping the .git directory.
func FromDir(dir string) (*Bundle, error) {
	b := &Bundle{Source: "rag-init at " + dir, Version: "local checkout", Files: map[string]File{}}
	root := os.DirFS(dir)
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		b.Files[p] = File{Data: data, Mode: info.Mode().Perm()}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	if _, ok := b.Files["AGENTS.md"]; !ok {
		return nil, fmt.Errorf("%s does not look like a rag-init checkout (no AGENTS.md)", dir)
	}
	return b, nil
}

// Load picks the source: a local directory, a downloaded ref, or the
// embedded snapshot, in that order of precedence.
func Load(ref, fromDir string) (*Bundle, error) {
	switch {
	case fromDir != "":
		return FromDir(fromDir)
	case ref != "":
		return Download(ref)
	default:
		return Embedded()
	}
}

// Under returns the paths below prefix (a slash-separated directory),
// sorted.
func (b *Bundle) Under(prefix string) []string {
	prefix = strings.TrimSuffix(prefix, "/") + "/"
	var out []string
	for p := range b.Files {
		if strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// Paths returns every path in the bundle, sorted.
func (b *Bundle) Paths() []string {
	out := make([]string, 0, len(b.Files))
	for p := range b.Files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// SkillNames returns the names of the skills under .agents/skills/.
func (b *Bundle) SkillNames() []string {
	seen := map[string]bool{}
	var names []string
	for _, p := range b.Under(".agents/skills") {
		name, _, _ := strings.Cut(strings.TrimPrefix(p, ".agents/skills/"), "/")
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// cleanRel rejects absolute paths and paths that leave the archive root, so a
// crafted archive cannot write outside the target directory.
func cleanRel(p string) (string, error) {
	c := path.Clean(p)
	if path.IsAbs(c) || c == ".." || strings.HasPrefix(c, "../") || filepath.IsAbs(filepath.FromSlash(c)) {
		return "", fmt.Errorf("unsafe path in archive: %q", p)
	}
	return c, nil
}
