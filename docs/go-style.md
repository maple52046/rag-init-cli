# Go style

How Go code is written in this repository. Where this file is silent, follow the
[Google Go Style Guide](https://google.github.io/styleguide/go/) (its Guide, Decisions, and Best Practices),
then [Effective Go](https://go.dev/doc/effective_go) and
[Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).

## Tooling

- Code is formatted with `gofmt`; CI fails on unformatted files.
- `go vet ./...` and `go test ./...` pass on Linux, macOS, and Windows before a change is merged.
- The module uses only the standard library. Adding a dependency needs a reason that the standard library
  cannot meet, written in the commit message.
- The `go` directive in `go.mod` stays at the oldest Go version the code needs, so `go install` works for
  people who do not run the latest release.

## Layout

- `cmd/rag-init/` holds `main` only: it wires the real environment (streams, terminal detection, process
  runner, version) into `internal/cli` and exits with its code.
- Behaviour lives in `internal/`: `content` (where rag-init files come from), `scaffold` (what to write),
  `tools` (external programs), `cli` (arguments and conversation with the user).
- Code that touches the outside world goes through an interface or a function value in `cli.Env` or
  `tools.Runner`, so tests can replace it. Tests never start real agents or call GitHub.

## Errors and output

- Return errors instead of panicking or exiting; only `main` exits. Wrap errors with context
  (`fmt.Errorf("download rag-init %s: %w", ref, err)`) and check them with `errors.Is` / `errors.As`.
- Error messages are lowercase, have no trailing punctuation, and say what to do next when the user can fix it
  ("not a terminal; pass --yes to apply the changes or --dry-run to only show them").
- Usage errors exit with code 2, other failures with code 1.
- Results go to standard output; warnings and errors go to standard error.

## Files and safety

- Never overwrite a user's file unless a flag asks for it. Plan first, show the plan, then apply.
- Build paths with `path` for slash-separated content paths and `path/filepath` for the file system; convert
  with `filepath.FromSlash`. Reject archive paths that are absolute or contain `..`.
- Read user input byte by byte where the same input stream is later handed to another process.

## Naming and comments

- Package names are short, lowercase, and say what the package provides (`scaffold`, not `utils`).
- Every exported identifier and every package has a doc comment that starts with its name.
- Comments explain intent, constraints, and non-obvious decisions; they do not restate the code.

## Tests

- Prefer table-driven tests and `t.TempDir()` for anything that writes files.
- Test behaviour that users rely on: existing files are kept, a second run changes nothing, flags combine as
  documented.
