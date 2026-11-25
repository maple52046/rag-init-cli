# Maintaining rag-init-cli

Rules for agents and people who change this repository: the `rag-init` command-line tool that lays out the
[rag-init](https://github.com/maple52046/rag-init) knowledge-base template.

## Before changing code

- Read [`docs/go-style.md`](docs/go-style.md) and follow it.
- Run `gofmt -l .`, `go vet ./...`, and `go test ./...`; all must be clean.

## Embedded rag-init content

- The binary embeds one tagged version of rag-init as `internal/content/snapshot/rag-init.tar.gz`, with the tag
  and commit in `internal/content/snapshot/ref.txt`. The content is kept as an archive on purpose: unpacked
  `SKILL.md` and `AGENTS.md` files inside this repository would be picked up by coding agents as skills and
  rules of this repository.
- Never edit the snapshot by hand. Replace it with `go run ./tools/sync-content -ref <rag-init tag>`.
- Embed tags only, not branches, in anything that is released.
- The CLI relies on rag-init's paths: `AGENTS.md` with the `knowledge-rules` markers, `CLAUDE.md`,
  `docs/knowledge-governance.md`, `knowledge/`, `knowledge/sources/`, `.agents/skills/<name>/`, and
  `.claude/skills/<name>/`. When rag-init changes any of them, update `internal/scaffold` and its tests in the
  same release.

## Releasing

1. Tag rag-init first (`vX.Y.Z`) if its content changed.
2. Run `go run ./tools/sync-content -ref <that tag>`, run the tests, and commit the snapshot.
3. Tag this repository `vX.Y.Z` and push the tag. The `release` workflow builds the binaries for Linux, macOS,
   and Windows with GoReleaser and publishes a GitHub release with checksums.

## Documentation

- `README.md` (English) and `README.zh-TW.md` (Traditional Chinese) say the same thing. Change both in the same
  commit, including the usage text when flags change.

## Commits

- Commit messages follow Conventional Commits 1.0.0 and are written in English.
- Never commit credentials, tokens, private keys, internal host names, or personal contact details.
