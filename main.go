package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// readmeAllowed lists every README filename variant docfetch pulls.
// Lowercase, exact match (see isReadmeFile). Only .md, only Chinese variants.
var readmeAllowed = []string{
	"readme.md",
	"readme.zh.md", "readme_zh.md", "readme-zh.md",
	"readme.zh-cn.md", "readme_zh_cn.md", "readme-zh-cn.md",
	"readme.zh-hans.md", "readme_zh_hans.md", "readme-zh-hans.md",
}

// docsDirs lists directory names treated as documentation (case-insensitive).
var docsDirs = []string{
	"docs", "doc", "wiki", "documentation", "manual", "guide",
}

// cliHandler is a slog.Handler printing plain, human-readable lines:
//   - info:  "docfetch: <msg>"
//   - debug: "docfetch: [debug] <msg>"
//   - warn:  "docfetch: 警告：<msg>"
//   - error: "docfetch: 错误：<msg>"
//
// Writes to w (stderr for the CLI).
type cliHandler struct {
	level slog.Level
	w     io.Writer
}

// newLogger builds a slog.Logger writing to w. verbose=true enables debug level.
func newLogger(w io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(&cliHandler{level: level, w: w})
}

func (h *cliHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

func (h *cliHandler) Handle(_ context.Context, r slog.Record) error {
	var prefix string
	switch r.Level {
	case slog.LevelDebug:
		prefix = "docfetch: [debug] "
	case slog.LevelWarn:
		prefix = "docfetch: 警告："
	case slog.LevelError:
		prefix = "docfetch: 错误："
	default:
		prefix = "docfetch: "
	}
	_, err := fmt.Fprintf(h.w, "%s%s\n", prefix, r.Message)
	return err
}

func (h *cliHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *cliHandler) WithGroup(_ string) slog.Handler      { return h }

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is the CLI entry point, returning a process exit code.
// Usage: docfetch [-o dir] [-v] <repo-url>
func run(args []string) int {
	fs := flag.NewFlagSet("docfetch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("o", "", "target directory (default: owner--repo)")
	verbose := fs.Bool("v", false, "verbose: debug-level output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: docfetch [-o dir] [-v] <repo-url>")
		return 2
	}
	slog.SetDefault(newLogger(os.Stderr, *verbose))
	rawURL := fs.Arg(0)

	owner, repo, err := parseRepoURL(rawURL)
	if err != nil {
		slog.Error(err.Error())
		return 1
	}

	dir := *out
	if dir == "" {
		dir = owner + "--" + repo
	}

	if exists, nonEmpty := dirState(dir); exists && nonEmpty {
		slog.Error(fmt.Sprintf("目标目录 %q 已存在且非空", dir))
		return 1
	}

	if _, err := exec.LookPath("git"); err != nil {
		slog.Error("PATH 中未找到 git（是否已安装？）")
		return 1
	}

	slog.Info(fmt.Sprintf("正在克隆 %s → %s", rawURL, dir))
	if err := cloneRepo(rawURL, dir); err != nil {
		slog.Error(err.Error())
		return 1
	}
	slog.Info("克隆完成")

	branch, err := defaultBranch(dir)
	if err != nil {
		slog.Error(err.Error())
		return 1
	}
	slog.Debug("默认分支：" + branch)

	entries, err := listRootEntries(dir)
	if err != nil {
		slog.Error(err.Error())
		return 1
	}

	var foundDirs, foundReadmes []string
	for _, e := range entries {
		switch {
		case e.isTree && isDocsDir(e.name):
			foundDirs = append(foundDirs, e.name)
		case !e.isTree && isReadmeFile(e.name):
			foundReadmes = append(foundReadmes, e.name)
		}
	}

	if len(foundDirs) == 0 && len(foundReadmes) == 0 {
		slog.Warn("仓库根目录未找到任何 docs/README 条目，工作树可能为空")
	} else {
		slog.Info(fmt.Sprintf("找到 %d 个文档目录、%d 个 README 文件", len(foundDirs), len(foundReadmes)))
	}

	matched := make([]string, 0, len(foundDirs)+len(foundReadmes))
	for _, d := range foundDirs {
		matched = append(matched, d+"/")
	}
	matched = append(matched, foundReadmes...)
	slog.Debug("根目录匹配：" + strings.Join(matched, "、"))

	patterns := buildSparsePatterns(foundDirs, foundReadmes)
	slog.Debug("sparse patterns：" + strings.Join(patterns, ", "))

	if err := sparseCheckout(dir, patterns); err != nil {
		slog.Error(err.Error())
		return 1
	}

	if err := checkoutBranch(dir, branch); err != nil {
		slog.Error(err.Error())
		return 1
	}

	slog.Info(fmt.Sprintf("完成：%s → %s（分支 %s，%d 个文档目录，%d 个 README 文件）",
		rawURL, dir, branch, len(foundDirs), len(foundReadmes)))
	return 0
}

// parseRepoURL extracts owner and repo from a git hosting platform URL.
// Supports https://<host>/<owner>/<repo> and git@<host>:<owner>/<repo>,
// with optional trailing .git and/or slashes. The last two path segments
// are used, so nested group paths (e.g. GitLab's group/subgroup/repo) parse
// to subgroup/repo. A leading "~" on the owner segment (sourcehut) is trimmed.
func parseRepoURL(raw string) (owner, repo string, err error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", errors.New("empty repo URL")
	}

	var rest string
	var ok bool
	switch {
	case strings.HasPrefix(s, "https://"):
		// Drop the host; keep everything after the first "/".
		_, rest, ok = strings.Cut(strings.TrimPrefix(s, "https://"), "/")
		if !ok {
			return "", "", fmt.Errorf("invalid repo URL: %q", raw)
		}
	case strings.HasPrefix(s, "git@"):
		// scp-like syntax: path follows the first ":".
		_, rest, ok = strings.Cut(s, ":")
		if !ok {
			return "", "", fmt.Errorf("invalid repo URL: %q", raw)
		}
	default:
		return "", "", fmt.Errorf("not a recognized repo URL: %q", raw)
	}

	// Strip trailing slash, .git, then a possible slash again.
	rest = strings.TrimSuffix(rest, "/")
	rest = strings.TrimSuffix(rest, ".git")
	rest = strings.TrimSuffix(rest, "/")

	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[len(parts)-2] == "" || parts[len(parts)-1] == "" {
		return "", "", fmt.Errorf("invalid repo URL: %q", raw)
	}
	owner = strings.TrimPrefix(parts[len(parts)-2], "~")
	repo = parts[len(parts)-1]
	return owner, repo, nil
}

// isReadmeFile reports whether name is a README variant docfetch pulls.
// Lowercases the name, then exact-matches against readmeAllowed.
func isReadmeFile(name string) bool {
	return slices.Contains(readmeAllowed, strings.ToLower(name))
}

// isDocsDir reports whether name is a documentation-like directory.
// Case-insensitive exact match against docsDirs.
func isDocsDir(name string) bool {
	return slices.Contains(docsDirs, strings.ToLower(name))
}

// buildSparsePatterns assembles git sparse-checkout (--no-cone) patterns.
// Directory names become "name/" (recurse into the dir); file names become
// "/name" (root-anchored). Names are used verbatim — preserve original case.
func buildSparsePatterns(dirNames, fileNames []string) []string {
	patterns := make([]string, 0, len(dirNames)+len(fileNames))
	for _, d := range dirNames {
		patterns = append(patterns, d+"/")
	}
	for _, f := range fileNames {
		patterns = append(patterns, "/"+f)
	}
	return patterns
}

// runGit executes git with args, optionally inside dir (cmd.Dir).
// git's stderr is passed through to the CLI's stderr in real time (clone
// progress etc.) while also being captured; on non-zero exit the returned
// error carries the captured stderr verbatim.
func runGit(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	slog.Debug("git: " + strings.Join(args, " "))
	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	out, err := cmd.Output()
	if err != nil {
		if stderr := strings.TrimSpace(stderrBuf.String()); stderr != "" {
			return out, fmt.Errorf("git %s: %s", strings.Join(args, " "), stderr)
		}
		return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// dirState reports whether dir exists and is non-empty (a file counts as
// non-empty).
func dirState(dir string) (exists, nonEmpty bool) {
	fi, err := os.Stat(dir)
	if err != nil {
		return false, false
	}
	if !fi.IsDir() {
		return true, true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true, true
	}
	return true, len(entries) > 0
}

// cloneRepo does a blobless, no-checkout clone into dir.
func cloneRepo(rawURL, dir string) error {
	_, err := runGit("", "clone", "--filter=blob:none", "--no-checkout", rawURL, dir)
	return err
}

// defaultBranch resolves origin/HEAD to a short branch name.
func defaultBranch(dir string) (string, error) {
	out, err := runGit(dir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		// Fallback: rev-parse the symbolic ref.
		out, err = runGit(dir, "rev-parse", "--abbrev-ref", "origin/HEAD")
		if err != nil {
			return "", err
		}
	}
	s := strings.TrimSpace(string(out))
	s = strings.TrimPrefix(s, "refs/remotes/origin/")
	s = strings.TrimPrefix(s, "origin/")
	return s, nil
}

// rootEntry is a single entry from `git ls-tree HEAD` at the repo root.
type rootEntry struct {
	name   string
	isTree bool
}

// listRootEntries lists the top-level tree entries of HEAD.
func listRootEntries(dir string) ([]rootEntry, error) {
	out, err := runGit(dir, "ls-tree", "HEAD")
	if err != nil {
		return nil, err
	}
	var entries []rootEntry
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// <mode> <type> <hash>\t<name>
		meta, name, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		entries = append(entries, rootEntry{name: name, isTree: strings.Contains(meta, " tree ")})
	}
	return entries, nil
}

// sparseCheckout configures --no-cone patterns and applies them.
func sparseCheckout(dir string, patterns []string) error {
	if _, err := runGit(dir, "sparse-checkout", "init", "--no-cone"); err != nil {
		return err
	}
	setArgs := append([]string{"sparse-checkout", "set"}, patterns...)
	_, err := runGit(dir, setArgs...)
	return err
}

// checkoutBranch materializes the working tree per the sparse-checkout config.
func checkoutBranch(dir, branch string) error {
	_, err := runGit(dir, "checkout", branch)
	return err
}
