package main

import (
	"errors"
	"fmt"
	"os"
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

func main() {
	fmt.Fprintln(os.Stderr, "docfetch: not yet implemented")
	os.Exit(2)
}
