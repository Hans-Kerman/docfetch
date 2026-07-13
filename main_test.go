package main

import "testing"

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
		{in: "https://github.com/Hans-Kerman/docfetch/tree/main/docs", wantOwner: "Hans-Kerman", wantRepo: "docfetch"},
		{in: "https://github.com/owner/repo/", wantOwner: "owner", wantRepo: "repo"},

		{in: "", wantErr: true},
		{in: "   ", wantErr: true},
		{in: "https://gitlab.com/owner/repo", wantErr: true},
		{in: "git@gitlab.com:owner/repo", wantErr: true},
		{in: "https://github.com/owner", wantErr: true},
		{in: "https://github.com/owner/", wantErr: true},
		{in: "https://github.com//repo", wantErr: true},
		{in: "not a url at all", wantErr: true},
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
