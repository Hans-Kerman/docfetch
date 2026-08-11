package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestParseRepoURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in        string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{in: "https://github.com/Hans-Kerman/docfetch", wantOwner: "Hans-Kerman", wantRepo: "docfetch"},
		{in: "https://github.com/Hans-Kerman/docfetch.git", wantOwner: "Hans-Kerman", wantRepo: "docfetch"},
		{in: "https://github.com/Hans-Kerman/docfetch/", wantOwner: "Hans-Kerman", wantRepo: "docfetch"},
		{in: "https://github.com/Hans-Kerman/docfetch.git/", wantOwner: "Hans-Kerman", wantRepo: "docfetch"},
		{in: "git@github.com:Hans-Kerman/docfetch.git", wantOwner: "Hans-Kerman", wantRepo: "docfetch"},
		{in: "git@github.com:Hans-Kerman/docfetch", wantOwner: "Hans-Kerman", wantRepo: "docfetch"},
		{in: "https://github.com/owner/repo/", wantOwner: "owner", wantRepo: "repo"},

		{in: "https://gitlab.com/group/repo.git", wantOwner: "group", wantRepo: "repo"},
		{in: "https://gitlab.com/group/subgroup/repo", wantOwner: "subgroup", wantRepo: "repo"},
		{in: "git@gitlab.com:owner/repo", wantOwner: "owner", wantRepo: "repo"},
		{in: "git@bitbucket.org:owner/repo", wantOwner: "owner", wantRepo: "repo"},
		{in: "https://codeberg.org/owner/repo", wantOwner: "owner", wantRepo: "repo"},
		{in: "https://git.sr.ht/~user/repo", wantOwner: "user", wantRepo: "repo"},
		{in: "git@git.sr.ht:~user/repo", wantOwner: "user", wantRepo: "repo"},

		{in: "", wantErr: true},
		{in: "   ", wantErr: true},
		{in: "not a url at all", wantErr: true},
		{in: "https://github.com", wantErr: true},
		{in: "https://github.com/", wantErr: true},
		{in: "https://github.com/owner", wantErr: true},
		{in: "https://github.com/owner/", wantErr: true},
		{in: "https://github.com//repo", wantErr: true},
		{in: "git@github.com:", wantErr: true},
		{in: "git@github.com:owner", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			owner, repo, err := parseRepoURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseRepoURL(%q) want error, got nil (%s/%s)", tc.in, owner, repo)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRepoURL(%q) unexpected error: %v", tc.in, err)
			}
			if owner != tc.wantOwner {
				t.Errorf("parseRepoURL(%q) owner = %q, want %q", tc.in, owner, tc.wantOwner)
			}
			if repo != tc.wantRepo {
				t.Errorf("parseRepoURL(%q) repo = %q, want %q", tc.in, repo, tc.wantRepo)
			}
		})
	}
}

func TestIsReadmeFile(t *testing.T) {
	t.Parallel()
	yes := []string{
		"README.md", "readme.md", "Readme.MD", "README.MD",
		"README.zh.md", "README_zh.md", "README-zh.md",
		"readme.zh-cn.md", "readme_zh_cn.md", "readme-zh-cn.md",
		"README.zh-Hans.md", "README_ZH_HANS.md", "readme-zh-Hans.md",
	}
	for _, name := range yes {
		if !isReadmeFile(name) {
			t.Errorf("isReadmeFile(%q) = false, want true", name)
		}
	}

	no := []string{
		"README.txt", "README", "readme.rst",
		"README.en.md", "readme.zh-tw.md", "readme.zh-TW.md",
		"README.zh.md.bak", "READMEs.md",
		"CONTRIBUTING.md", "CHANGELOG.md", "docs.md",
	}
	for _, name := range no {
		if isReadmeFile(name) {
			t.Errorf("isReadmeFile(%q) = true, want false", name)
		}
	}
}

func TestIsDocsDir(t *testing.T) {
	t.Parallel()
	yes := []string{
		"docs", "doc", "wiki", "documentation", "manual", "guide",
		"Docs", "DOCS", "Doc", "WIKI", "Documentation", "MANUAL", "Guide",
	}
	for _, name := range yes {
		if !isDocsDir(name) {
			t.Errorf("isDocsDir(%q) = false, want true", name)
		}
	}

	no := []string{
		"docsrc", "mydocs", "documents", "docx", "docs-extra", "readme",
		".docs", "docs2", "api", "src", "examples",
	}
	for _, name := range no {
		if isDocsDir(name) {
			t.Errorf("isDocsDir(%q) = true, want false", name)
		}
	}
}

func TestBuildSparsePatterns(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		dirNames  []string
		fileNames []string
		want      []string
	}{
		{
			name:      "dirs and files",
			dirNames:  []string{"docs", "wiki"},
			fileNames: []string{"README.md", "README.zh.md"},
			want:      []string{"docs/", "wiki/", "/README.md", "/README.zh.md"},
		},
		{
			name:      "only files",
			dirNames:  nil,
			fileNames: []string{"README.md"},
			want:      []string{"/README.md"},
		},
		{
			name:      "only dirs",
			dirNames:  []string{"docs"},
			fileNames: nil,
			want:      []string{"docs/"},
		},
		{
			name:      "empty",
			dirNames:  nil,
			fileNames: nil,
			want:      []string{},
		},
		{
			name:      "preserve case",
			dirNames:  []string{"Docs"},
			fileNames: []string{"README.zh-CN.md"},
			want:      []string{"Docs/", "/README.zh-CN.md"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildSparsePatterns(tc.dirNames, tc.fileNames)
			if len(got) != len(tc.want) {
				t.Fatalf("buildSparsePatterns len = %d, want %d (got %v)", len(got), len(tc.want), got)
			}
			for i, p := range got {
				if p != tc.want[i] {
					t.Errorf("buildSparsePatterns[%d] = %q, want %q", i, p, tc.want[i])
				}
			}
		})
	}
}

func TestNewLoggerDefault(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf, false)

	logger.Info("正在克隆 https://example.com/repo → dir")
	logger.Warn("仓库根目录未找到任何 docs/README 条目，工作树可能为空")
	logger.Error("clone failed: 无法解析主机")
	logger.Debug("默认分支：main")

	want := "docfetch: 正在克隆 https://example.com/repo → dir\n" +
		"docfetch: 警告：仓库根目录未找到任何 docs/README 条目，工作树可能为空\n" +
		"docfetch: 错误：clone failed: 无法解析主机\n"
	if got := buf.String(); got != want {
		t.Errorf("default logger output = %q, want %q", got, want)
	}
}

func TestNewLoggerVerbose(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf, true)

	logger.Debug("默认分支：main")
	logger.Info("克隆完成")

	want := "docfetch: [debug] 默认分支：main\n" +
		"docfetch: 克隆完成\n"
	if got := buf.String(); got != want {
		t.Errorf("verbose logger output = %q, want %q", got, want)
	}
}

func TestRunGitPassesStderrThrough(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
		w.Close()
	}()

	_, _ = runGit("", "definitely-not-a-real-git-command")

	os.Stderr = oldStderr
	w.Close()
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "definitely-not-a-real-git-command") {
		t.Errorf("stderr passthrough = %q, want git's stderr text", out)
	}
}
