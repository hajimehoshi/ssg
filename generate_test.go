// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package ssg_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hajimehoshi/ssg"
)

func TestGenerateResourceVersionQuery(t *testing.T) {
	dir := t.TempDir()
	inDir := filepath.Join(dir, "src", "content")
	layoutDir := filepath.Join(dir, "src", "layouts")
	outDir := filepath.Join(dir, "public")
	for _, path := range []string{inDir, layoutDir} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		"index.html": `<p>hello</p>`,
		"site.css":   `body { color: red; }`,
		"unused.bin": `unused`,
	} {
		if err := os.WriteFile(filepath.Join(inDir, path), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(layoutDir, "default.html"), []byte(`<html><head><link rel="stylesheet" href="/site.css"/></head><body>{{.Page.Content}}</body></html>`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ssg.Generate(&ssg.GenerateOptions{
		Dir:      dir,
		SiteName: "Test",
	}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`href="/site\.css\?v=([a-z2-7]{10})"`).FindSubmatch(content)
	if match == nil {
		t.Fatalf("generated HTML has no stylesheet version query: %q", content)
	}
	original, err := os.ReadFile(filepath.Join(outDir, "site.css"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(original), "body{color:red}"; got != want {
		t.Errorf("stylesheet: got: %q, want: %q", got, want)
	}
	matches, err := filepath.Glob(filepath.Join(outDir, "site.*.css"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("stylesheet has fingerprinted copies: %q", matches)
	}
	if _, err := os.Stat(filepath.Join(outDir, "unused.bin")); err != nil {
		t.Errorf("unreferenced asset: %v", err)
	}
}

func TestGenerateSelectsLayout(t *testing.T) {
	dir := t.TempDir()
	contentDir := filepath.Join(dir, "src", "content")
	layoutDir := filepath.Join(dir, "src", "layouts")
	outputDir := filepath.Join(dir, "public")
	for _, path := range []string{
		filepath.Join(contentDir, "writings"),
		filepath.Join(layoutDir, "writings"),
	} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(contentDir, "index.html"): `<p>home</p>`,
		filepath.Join(contentDir, "normalized.html"): `<script type="application/yaml">
_layout: writings/../default
</script><p>normalized</p>`,
		filepath.Join(contentDir, "writings", "index.html"): `<script type="application/yaml">
_layout: writings/article
</script><p>writings</p>`,
		filepath.Join(layoutDir, "default.html"):             `<html><body><main>{{.Page.Content}}</main></body></html>`,
		filepath.Join(layoutDir, "writings", "article.html"): `<html><body><article>{{if index .Page.Meta "_layout"}}unexpected{{end}}{{.Page.Content}}</article></body></html>`,
		filepath.Join(layoutDir, "ignored.txt"):              `not a layout`,
	} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := ssg.Generate(&ssg.GenerateOptions{
		Dir:      dir,
		SiteName: "Test",
	}); err != nil {
		t.Fatal(err)
	}
	for path, marker := range map[string]string{
		"index.html":                            "<main>",
		"normalized.html":                       "<main>",
		filepath.Join("writings", "index.html"): "<article>",
	} {
		content, err := os.ReadFile(filepath.Join(outputDir, path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), marker) {
			t.Errorf("%s does not contain %q: %q", path, marker, content)
		}
		if strings.Contains(string(content), "unexpected") {
			t.Errorf("%s received _layout in page metadata: %q", path, content)
		}
	}
}

func TestGenerateRejectsInvalidLayout(t *testing.T) {
	testCases := []struct {
		Name        string
		Layout      string
		Err         string
		OutsidePath string
	}{
		{
			Name:   "not a string",
			Layout: "3",
			Err:    "_layout for index.html must be a non-empty string",
		},
		{
			Name:   "empty",
			Layout: `""`,
			Err:    "_layout for index.html must be a non-empty string",
		},
		{
			Name:        "parent traversal",
			Layout:      "../article",
			Err:         `layout path "../article" for index.html is outside the layouts directory`,
			OutsidePath: filepath.Join("src", "article.html"),
		},
		{
			Name:   "absolute",
			Layout: "/article",
			Err:    `layout path "/article" for index.html must be relative`,
		},
		{
			Name:   "backslash",
			Layout: `"blog\\article"`,
			Err:    `layout path "blog\\article" for index.html must use forward slashes`,
		},
		{
			Name:   "missing",
			Layout: "missing",
			Err:    `layout "missing" for index.html not found`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			dir := t.TempDir()
			contentDir := filepath.Join(dir, "src", "content")
			layoutDir := filepath.Join(dir, "src", "layouts")
			for _, path := range []string{contentDir, layoutDir} {
				if err := os.MkdirAll(path, 0755); err != nil {
					t.Fatal(err)
				}
			}
			content := `<script type="application/yaml">_layout: ` + tc.Layout + `</script><p>hello</p>`
			if err := os.WriteFile(filepath.Join(contentDir, "index.html"), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(layoutDir, "default.html"), []byte(`<html><body>{{.Page.Content}}</body></html>`), 0644); err != nil {
				t.Fatal(err)
			}
			if tc.OutsidePath != "" {
				if err := os.WriteFile(filepath.Join(dir, tc.OutsidePath), []byte(`outside`), 0644); err != nil {
					t.Fatal(err)
				}
			}

			err := ssg.Generate(&ssg.GenerateOptions{
				Dir:      dir,
				SiteName: "Test",
			})
			if err == nil || !strings.Contains(err.Error(), tc.Err) {
				t.Errorf("Generate: got: %v, want an error containing %q", err, tc.Err)
			}
		})
	}
}

func TestGenerateRejectsLayoutSymlinkOutsideLayoutDir(t *testing.T) {
	dir := t.TempDir()
	contentDir := filepath.Join(dir, "src", "content")
	layoutDir := filepath.Join(dir, "src", "layouts")
	for _, path := range []string{contentDir, layoutDir} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	content := `<script type="application/yaml">_layout: external</script><p>hello</p>`
	if err := os.WriteFile(filepath.Join(contentDir, "index.html"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	outsideDir := filepath.Join(dir, "src", "layouts-other")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(outsideDir, "external.html")
	if err := os.WriteFile(outsidePath, []byte(`<html><body>{{.Page.Content}}</body></html>`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(layoutDir, "external.html")); err != nil {
		t.Fatal(err)
	}

	err := ssg.Generate(&ssg.GenerateOptions{
		Dir:      dir,
		SiteName: "Test",
	})
	want := `layout path "external" for index.html is outside the layouts directory`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Generate: got: %v, want an error containing %q", err, want)
	}
}
