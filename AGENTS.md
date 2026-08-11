# AGENTS.md

## Project

**docfetch** — Go CLI to clone only a git hosting platform repo's documentation (docs/directory + README i18n variants) via `git sparse-checkout` + `--filter=blob:none`.

- Module: `github.com/Hans-Kerman/docfetch` / Go ≥ 1.26.5
- Commands: `docfetch [-o dir] [-v] <repo-url>` (q.v. design section for flow)
- No external dependencies (exec `git` directly; not go-git nor GitHub API)
- Single-file project: all logic in `main.go`; tests in `main_test.go`
- Uses Go 1.26 idioms: `strings.SplitSeq` for line iteration; stderr capture via `io.MultiWriter`

## Design

Data flow:
1. Parse repo URL → owner, repo
2. Compute target dir: `-o` flag else `owner--repo`
3. `git clone --filter=blob:none --no-checkout <url> <dir>`
4. Determine default branch: `git symbolic-ref origin/HEAD`
5. `git ls-tree HEAD` → list root entries (tree already fetched via blob:none)
6. Filter root entries: README variants + docs-like dirs
7. `git sparse-checkout init --no-cone`
8. `git sparse-checkout set <patterns>` (gitignore syntax: `docs/`, `/README.md`)
9. `git checkout <branch>`
10. Working tree: only docs dirs + README files + `.git`

### Logging

- All user-facing output via `log/slog` (stdlib, no deps) → stderr:
  info `docfetch: <msg>` / debug `docfetch: [debug] <msg>` / warn `docfetch: 警告：…` / error `docfetch: 错误：…`, messages in Chinese
- Default level Info (key steps); `-v` flag enables Debug (branch, matched
  entries, sparse patterns, per-git-command echo)
- git stderr is always passed through in real time (clone progress visible),
  and simultaneously captured for error messages
- Usage/flag-parse errors stay as plain stderr (logger not set up yet)

### Components (independent, table-testable)

1. **`main.go`** — CLI entry, orchestration
2. **Pure functions** (table-driven tests):
   - `parseRepoURL` — supports `https://<host>/` and `git@<host>:` URLs, trailing `.git`, slashes
   - `isReadmeFile` — lowercase name, exact match against allowed list (see below)
   - `isDocsDir` — matches `docs`, `doc`, `wiki`, `documentation`, `manual`, `guide` (case-insensitive)
   - `buildSparsePatterns` — assembles sparse-checkout patterns (dirs: `docs/`; files: `/README.md`)
3. **`runGit(dir, args...)`** — wraps `exec.Command("git", args...)` with stderr capture

### README matching

Lowercase the filename, exact match against:

```
readme.md
readme.zh.md      readme_zh.md      readme-zh.md
readme.zh-cn.md   readme_zh_cn.md   readme-zh-cn.md
readme.zh-hans.md readme_zh_hans.md readme-zh-hans.md
```

Only `.md` extension. Only Chinese variants (zh, zh-CN, zh-Hans). No regex.

### Error / edge cases

- `-o` dir exists & non-empty → error
- git not installed → clear error
- Private/non-existent repo → pass through git stderr
- URL parse failure → error
- Zero README/docs entries found → warn, sparse-checkout still executed (potentially empty working tree)

### YAGNI (v1 not)

- `pull`/update subcommand (v1 is fresh-clone only)
- Credential caching

## Commands

- Build: `go build -o docfetch .`
- Run: `go run . [-o dir] <repo-url>`
- Test: `go test ./...`
- Test single function with verbose: `go test -run TestParseRepoURL -v ./...`
- Lint (if present): `golangci-lint run ./...`
- Format: `go fmt ./...`
