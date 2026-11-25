# rag-init-cli

`rag-init` sets up a knowledge base that coding agents read and write, from the
[rag-init](https://github.com/maple52046/rag-init) template, and then hands over to your agent to tailor the
rules to your situation.

[繁體中文說明](README.zh-TW.md)

You do not need this tool to use rag-init: you can also click **Use this template** on GitHub or copy the
skills by hand (see the [rag-init README](https://github.com/maple52046/rag-init#three-ways-to-use-it)). The
CLI does the same in one command and works offline.

## Install

Download the archive for your system from the [releases page](https://github.com/maple52046/rag-init-cli/releases),
check it against `checksums.txt`, and put `rag-init` (`rag-init.exe` on Windows) on your `PATH`.

Or build it with Go 1.24 or later:

```sh
go install github.com/maple52046/rag-init-cli/cmd/rag-init@latest
```

The release binaries are not code-signed. On macOS, remove the quarantine flag after downloading
(`xattr -d com.apple.quarantine rag-init`); on Windows, SmartScreen may ask you to confirm ("More info" →
"Run anyway").

## Start a new knowledge base

```sh
rag-init new my-knowledge-base
```

This creates the directory with the same files as the template (the default rules, the `knowledge/`
skeleton, the methodology sources, and both skills) and a short `README.md`, runs `git init` on branch `main`,
and commits everything. Then it offers to start an installed agent (Claude Code, Codex, Cursor CLI, Antigravity
CLI, or Gemini CLI), which interviews you and rewrites the rules.

Create the GitHub repository at the same time (requires the [GitHub CLI](https://cli.github.com/), logged in):

```sh
rag-init new my-knowledge-base --github your-name/my-knowledge-base            # private
rag-init new my-knowledge-base --github your-name/my-knowledge-base --public
```

## Add rag-init to an existing project

```sh
cd your-project
rag-init init
```

`init` never overwrites your files. It adds:

- the `create-knowledge-rules` and `git-commit` skills to `.agents/skills/`, with Claude Code wrappers in
  `.claude/skills/`;
- the `knowledge-rules` section to `AGENTS.md` (creating the file if needed, appending to it otherwise);
- `@AGENTS.md` to `CLAUDE.md`, so Claude Code reads the same rules;
- `docs/knowledge-governance.md` and the `knowledge/` index, taxonomy, and glossary, when they are missing;
- with `--with-sources`, the methodology sources in `knowledge/sources/` and their license notices.

Run it again later with `--update-skills` to replace the two skills with the version in the CLI. Your rules
are never touched; to change them, run the `create-knowledge-rules` skill.

## Flags

| Flag | Commands | Meaning |
| --- | --- | --- |
| `--agent <name>` | new, init | Hand over to `claude`, `codex`, `cursor`, `antigravity`, or `gemini` without asking; `none` skips the handover |
| `--gemini` | new, init | Make Gemini CLI read `AGENTS.md` by adding it to `context.fileName` in `.gemini/settings.json` |
| `--ref <ref>` | new, init | Download this rag-init tag, branch, or commit instead of using the embedded version |
| `--from <dir>` | new, init | Use a local rag-init checkout instead of the embedded version |
| `--dry-run` | new, init | Show the planned changes and stop |
| `-y`, `--yes` | new, init | Apply without asking; required when not running in a terminal |
| `--github <repo>` | new | Also create the GitHub repository with `gh` and push |
| `--public` | new | Make that repository public (default: private) |
| `--no-git` | new | Skip `git init` and the first commit |
| `--with-sources` | init | Also copy the methodology sources |
| `--update-skills` | init | Replace existing copies of the two skills |

`rag-init version` prints the CLI version and the rag-init version embedded in it.

## Handing over to an agent

After setup, `rag-init` starts the agent you pick in the target directory with this first message:
"Use the create-knowledge-rules skill to tailor this repository's knowledge rules to my situation." The
commands it runs are `claude <prompt>`, `codex <prompt>`, `cursor-agent <prompt>` (or `agent <prompt>`),
`agy -i <prompt>`, and `gemini -i <prompt>`. Without a terminal, or when no supported agent is installed, it
prints what to do instead.

## License

MIT, see [`LICENSE`](LICENSE). The rag-init content embedded in the binary carries its own notices; see
rag-init's [`THIRD_PARTY_NOTICES.md`](https://github.com/maple52046/rag-init/blob/main/THIRD_PARTY_NOTICES.md).
