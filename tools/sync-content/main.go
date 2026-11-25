// Command sync-content replaces the embedded rag-init snapshot with the
// GitHub archive of a ref.
//
// Usage:
//
//	go run ./tools/sync-content -ref v0.1.0
//
// It writes internal/content/snapshot/rag-init.tar.gz and
// internal/content/snapshot/ref.txt ("<ref> <commit>"). Run it from the
// repository root.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/maple52046/rag-init-cli/internal/content"
)

func main() {
	ref := flag.String("ref", "", "rag-init tag, branch, or commit to embed (required)")
	out := flag.String("out", filepath.Join("internal", "content", "snapshot"), "snapshot directory")
	flag.Parse()
	if *ref == "" {
		fmt.Fprintln(os.Stderr, "sync-content: -ref is required, for example -ref v0.1.0")
		os.Exit(2)
	}
	if err := run(*ref, *out); err != nil {
		fmt.Fprintln(os.Stderr, "sync-content:", err)
		os.Exit(1)
	}
}

func run(ref, out string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(content.ArchiveURL(ref))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", content.ArchiveURL(ref), resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	commit, err := archiveCommit(data)
	if err != nil {
		return err
	}
	if _, err := content.FromArchive(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("archive is not usable: %w", err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "rag-init.tar.gz"), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "ref.txt"), []byte(ref+" "+commit+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("embedded rag-init %s (%s), %d bytes\n", ref, commit, len(data))
	return nil
}

// archiveCommit reads the commit SHA that GitHub records in the archive's pax
// global header.
func archiveCommit(data []byte) (string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return "", errors.New("archive has no pax global header with the commit")
		}
		if err != nil {
			return "", err
		}
		if c := hdr.PAXRecords["comment"]; c != "" {
			return c, nil
		}
	}
}
