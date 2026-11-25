// Command rag-init sets up a knowledge base that coding agents read and
// write, from the rag-init template, and hands over to an agent that tailors
// the rules.
package main

import (
	"os"
	"runtime/debug"

	"github.com/maple52046/rag-init-cli/internal/cli"
	"github.com/maple52046/rag-init-cli/internal/content"
	"github.com/maple52046/rag-init-cli/internal/tools"
)

// version is set by the release build (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.Env{
		In:      os.Stdin,
		Out:     os.Stdout,
		Err:     os.Stderr,
		IsTTY:   isTerminal(os.Stdin) && isTerminal(os.Stdout),
		Runner:  tools.System{},
		Load:    content.Load,
		Version: buildVersion(),
	}))
}

// buildVersion falls back to the module version that `go install ...@vX`
// records, so installs without the release ldflags still report a version.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
