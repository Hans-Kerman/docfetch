package main

import (
	"errors"
	"flag"
	"fmt"
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

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is the CLI entry point, returning a process exit code.
// Usage: docfetch [-o dir] <github-url>
func run(args []string) int {
	fs := flag.NewFlagSet("docfetch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("o", "", "target directory (default: owner--repo)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: docfetch [-o dir] <github-url>")
		return 2
	}
	rawURL := fs.Arg(0)

	owner, repo, err := parseRepoURL(rawURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docfetch: %v\n", err)
		return 1
	}

	dir := *out
	if dir == "" {
		dir = owner + "--" + repo
	}

	if exists, nonEmpty := dirState(dir); exists && nonEmpty {
		fmt.Fprintf(os.Stderr, "docfetch: target directory %q exists and is non-empty\n", dir)
		return 1
	}

	if _, err := exec.LookPath("git"); err != nil {
		fmt.Fprintln(os.Stderr, "docfetch: git not found in PATH (is git installed?)")
		return 1
	}

	if err := cloneRepo(rawURL, dir); err != nil {
		fmt.Fprintf(os.Stderr, "docfetch: clone failed: %v\n", err)
		return 1
	}

	branch, err := defaultBranch(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docfetch: cannot determine default branch: %v\n", err)
		return 1
	}

	entries, err := listRootEntries(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docfetch: listing root entries failed: %v\n", err)
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
		fmt.Fprintln(os.Stderr, "docfetch: warning: no README or docs entries found at repo root")
	}

	patterns := buildSparsePatterns(foundDirs, foundReadmes)

	if err := sparseCheckout(dir, patterns); err != nil {
		fmt.Fprintf(os.Stderr, "docfetch: sparse-checkout failed: %v\n", err)
		return 1
	}

	if err := checkoutBranch(dir, branch); err != nil {
		fmt.Fprintf(os.Stderr, "docfetch: checkout failed: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "docfetch: %s -> %s (branch %s): %d doc dir(s), %d README file(s)\n",
		rawURL, dir, branch, len(foundDirs), len(foundReadmes))
	return 0
}

// parseRepoURL extracts owner and repo from a GitHub URL.
// Supports https://github.com/<owner>/<repo> and git@github.com:<owner>/<repo>,
// with optional trailing .git and/or slashes. Extra path segments are ignored.
func parseRepoURL(raw string) (owner, repo string, err error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", errors.New("empty repo URL")
	}

	var rest string
	switch {
	case strings.HasPrefix(s, "https://github.com/"):
		rest = strings.TrimPrefix(s, "https://github.com/")
	case strings.HasPrefix(s, "git@github.com:"):
		rest = strings.TrimPrefix(s, "git@github.com:")
	default:
		return "", "", fmt.Errorf("not a GitHub URL: %q", raw)
	}

	// Strip trailing slash, .git, then a possible slash again.
	rest = strings.TrimSuffix(rest, "/")
	rest = strings.TrimSuffix(rest, ".git")
	rest = strings.TrimSuffix(rest, "/")

	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid GitHub URL: %q", raw)
	}
	return parts[0], parts[1], nil
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
// On non-zero exit it returns an error carrying git's stderr verbatim.
func runGit(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			if stderr := strings.TrimSpace(string(ee.Stderr)); stderr != "" {
				return out, fmt.Errorf("git %s: %s", strings.Join(args, " "), stderr)
			}
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
