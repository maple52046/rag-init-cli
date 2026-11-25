# rag-init-cli

`rag-init` 用 [rag-init](https://github.com/maple52046/rag-init) template 建立一個給 coding agent 讀寫的知識庫，
接著交棒給你的 agent，依你的情境調整規則。

[English](README.md)

使用 rag-init 不一定需要這個工具：也可以在 GitHub 按 **Use this template**，或手動複製 skill（見
[rag-init 的 README](https://github.com/maple52046/rag-init/blob/main/README.zh-TW.md)）。CLI 用一個指令做到同樣的事，
而且不需要網路。

## 安裝

到 [releases 頁面](https://github.com/maple52046/rag-init-cli/releases) 下載適合你系統的壓縮檔，對照
`checksums.txt` 檢查後，把 `rag-init`（Windows 為 `rag-init.exe`）放進 `PATH`。

或用 Go 1.24 以上版本自行建置：

```sh
go install github.com/maple52046/rag-init-cli/cmd/rag-init@latest
```

發布的 binary 沒有程式碼簽章。macOS 下載後請移除隔離標記（`xattr -d com.apple.quarantine rag-init`）；Windows 的
SmartScreen 可能要求確認（「其他資訊」→「仍要執行」）。

## 建立新的知識庫

```sh
rag-init new my-knowledge-base
```

這會建立目錄，內容與 template 相同（預設規則、`knowledge/` 骨架、方法論來源和兩個 skill），另附一份簡短的
`README.md`，然後在 `main` 分支執行 `git init` 並 commit 全部檔案。最後會詢問要不要啟動已安裝的 agent（Claude Code、
Codex、Cursor CLI、Antigravity CLI 或 Gemini CLI），由它訪談你並改寫規則。

同時建立 GitHub repository（需要已登入的 [GitHub CLI](https://cli.github.com/)）：

```sh
rag-init new my-knowledge-base --github your-name/my-knowledge-base            # 私有
rag-init new my-knowledge-base --github your-name/my-knowledge-base --public
```

## 加進既有專案

```sh
cd your-project
rag-init init
```

`init` 不會覆寫你的檔案。它會加入：

- `create-knowledge-rules` 和 `git-commit` 兩個 skill，放在 `.agents/skills/`，並在 `.claude/skills/` 放 Claude Code
  的 wrapper；
- `AGENTS.md` 的 `knowledge-rules` 段落（沒有檔案就建立，已有檔案就附加在最後）；
- 在 `CLAUDE.md` 加上 `@AGENTS.md`，讓 Claude Code 讀同一份規則；
- 缺少時補上 `docs/knowledge-governance.md` 和 `knowledge/` 的 index、taxonomy、glossary；
- 加 `--with-sources` 時，另外複製 `knowledge/sources/` 的方法論來源與授權聲明。

之後可以加 `--update-skills` 再執行一次，把兩個 skill 換成 CLI 內建的版本。你的規則永遠不會被改動；要調整規則，
請執行 `create-knowledge-rules` skill。

## 參數

| 參數 | 適用指令 | 說明 |
| --- | --- | --- |
| `--agent <name>` | new、init | 不詢問，直接交棒給 `claude`、`codex`、`cursor`、`antigravity` 或 `gemini`；`none` 表示不交棒 |
| `--gemini` | new、init | 在 `.gemini/settings.json` 的 `context.fileName` 加入 `AGENTS.md`，讓 Gemini CLI 讀它 |
| `--ref <ref>` | new、init | 下載這個 rag-init tag、branch 或 commit，而不用內建版本 |
| `--from <dir>` | new、init | 使用本機的 rag-init checkout，而不用內建版本 |
| `--dry-run` | new、init | 只列出預計的變更，不寫入 |
| `-y`、`--yes` | new、init | 不詢問直接套用；不在終端機執行時必須加 |
| `--github <repo>` | new | 同時用 `gh` 建立 GitHub repository 並 push |
| `--public` | new | 該 repository 設為公開（預設私有） |
| `--no-git` | new | 不執行 `git init` 和第一個 commit |
| `--with-sources` | init | 一併複製方法論來源 |
| `--update-skills` | init | 取代已存在的兩個 skill |

`rag-init version` 會列出 CLI 版本和內建的 rag-init 版本。

## 交棒給 agent

鋪設完成後，`rag-init` 會在目標目錄啟動你選的 agent，第一句訊息是：「Use the create-knowledge-rules skill to
tailor this repository's knowledge rules to my situation.」實際執行的指令分別是 `claude <prompt>`、
`codex <prompt>`、`cursor-agent <prompt>`（或 `agent <prompt>`）、`agy -i <prompt>` 與 `gemini -i <prompt>`。不在
終端機執行，或沒有安裝任何支援的 agent 時，會改為印出接下來該怎麼做。

## 授權

MIT，見 [`LICENSE`](LICENSE)。binary 內建的 rag-init 內容附有各自的授權聲明，見 rag-init 的
[`THIRD_PARTY_NOTICES.md`](https://github.com/maple52046/rag-init/blob/main/THIRD_PARTY_NOTICES.md)。
