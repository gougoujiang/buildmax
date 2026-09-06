package architecture_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const translationNoticeFormat = "> **翻译说明：** 本文是[英文原文](%s)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `%x`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。"

// TestDesignTranslationsMirrorEnglish makes the English design tree the only
// synchronization authority. The Chinese mirror stores only a digest of the
// exact English source it translated, so a source edit, addition, or removal
// cannot silently leave a translation looking current.
func TestDesignTranslationsMirrorEnglish(t *testing.T) {
	root := repoRoot(t)
	problems := designTranslationProblems(
		t,
		filepath.Join(root, "docs", "design"),
		filepath.Join(root, "docs", "zh-CN", "design"),
	)
	for _, problem := range problems {
		t.Error(problem)
	}
}

func TestDesignTranslationDriftDetection(t *testing.T) {
	t.Run("synchronized", func(t *testing.T) {
		en, zh := translationFixture(t)
		writeTranslationPair(t, en, zh, "nested/record.md", "# Decision\n\nCurrent source.\n")
		if problems := designTranslationProblems(t, en, zh); len(problems) != 0 {
			t.Fatalf("unexpected problems: %v", problems)
		}
	})

	for _, test := range []struct {
		name string
		edit func(t *testing.T, en, zh string)
		want string
	}{
		{
			name: "missing mirror",
			edit: func(t *testing.T, en, _ string) {
				writeFile(t, filepath.Join(en, "missing.md"), "# Missing\n")
			},
			want: "missing Chinese mirror",
		},
		{
			name: "stale mirror",
			edit: func(t *testing.T, en, zh string) {
				writeTranslationPair(t, en, zh, "record.md", "# Decision\n\nCurrent source.\n")
				writeFile(t, filepath.Join(en, "record.md"), "# Decision\n\nChanged source.\n")
			},
			want: "missing current synchronization notice",
		},
		{
			name: "orphaned mirror",
			edit: func(t *testing.T, _, zh string) {
				writeFile(t, filepath.Join(zh, "retired.md"), "# Retired\n")
			},
			want: "has no English source",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			en, zh := translationFixture(t)
			test.edit(t, en, zh)
			problems := strings.Join(designTranslationProblems(t, en, zh), "\n")
			if !strings.Contains(problems, test.want) {
				t.Fatalf("problems %q do not contain %q", problems, test.want)
			}
		})
	}
}

func designTranslationProblems(t *testing.T, englishDir, chineseDir string) []string {
	t.Helper()
	english := markdownTree(t, englishDir)
	chinese := markdownTree(t, chineseDir)
	var problems []string

	for rel, englishPath := range english {
		chinesePath, ok := chinese[rel]
		if !ok {
			problems = append(problems, fmt.Sprintf("docs/design/%s: missing Chinese mirror docs/zh-CN/design/%s", rel, rel))
			continue
		}
		body, err := os.ReadFile(englishPath)
		if err != nil {
			t.Fatalf("read %s: %v", englishPath, err)
		}
		chineseBody, err := os.ReadFile(chinesePath)
		if err != nil {
			t.Fatalf("read %s: %v", chinesePath, err)
		}
		chineseLink, err := filepath.Rel(filepath.Dir(englishPath), chinesePath)
		if err != nil {
			t.Fatalf("relative Chinese link for %s: %v", rel, err)
		}
		englishNav := fmt.Sprintf("> **简体中文：** [阅读中文镜像](%s)", filepath.ToSlash(chineseLink))
		if !strings.Contains(string(body), englishNav) {
			problems = append(problems, fmt.Sprintf("docs/design/%s: missing Chinese navigation %q", rel, englishNav))
		}
		englishLink, err := filepath.Rel(filepath.Dir(chinesePath), englishPath)
		if err != nil {
			t.Fatalf("relative English link for %s: %v", rel, err)
		}
		notice := fmt.Sprintf(translationNoticeFormat, filepath.ToSlash(englishLink), sha256.Sum256(body))
		if !strings.Contains(string(chineseBody), notice) {
			problems = append(problems, fmt.Sprintf("docs/zh-CN/design/%s: missing current synchronization notice; retranslate docs/design/%s and update its source digest", rel, rel))
		}
	}
	for rel := range chinese {
		if _, ok := english[rel]; !ok {
			problems = append(problems, fmt.Sprintf("docs/zh-CN/design/%s has no English source; remove it or restore docs/design/%s", rel, rel))
		}
	}
	sort.Strings(problems)
	return problems
}

func markdownTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = path
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return files
}

func translationFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	en := filepath.Join(root, "docs", "design")
	zh := filepath.Join(root, "docs", "zh-CN", "design")
	if err := os.MkdirAll(en, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(zh, 0o755); err != nil {
		t.Fatal(err)
	}
	return en, zh
}

func writeTranslationPair(t *testing.T, englishDir, chineseDir, rel, source string) {
	t.Helper()
	englishPath := filepath.Join(englishDir, filepath.FromSlash(rel))
	chinesePath := filepath.Join(chineseDir, filepath.FromSlash(rel))
	chineseLink, err := filepath.Rel(filepath.Dir(englishPath), chinesePath)
	if err != nil {
		t.Fatal(err)
	}
	source = strings.Replace(source, "\n", "\n\n> **简体中文：** [阅读中文镜像]("+filepath.ToSlash(chineseLink)+")\n", 1)
	writeFile(t, englishPath, source)
	englishLink, err := filepath.Rel(filepath.Dir(chinesePath), englishPath)
	if err != nil {
		t.Fatal(err)
	}
	notice := fmt.Sprintf(translationNoticeFormat, filepath.ToSlash(englishLink), sha256.Sum256([]byte(source)))
	writeFile(t, chinesePath, "# 决定\n\n"+notice+"\n")
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
