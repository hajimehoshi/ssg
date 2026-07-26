// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package ssg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ssg"
)

func TestPageContentTitle(t *testing.T) {
	testCases := []struct {
		Name      string
		Extension string
		Content   string
		Title     string
	}{
		{
			Name:      "first HTML heading",
			Extension: ".html",
			Content:   `<h1>First</h1><h1>Second</h1>`,
			Title:     "First",
		},
		{
			Name:      "descendant text",
			Extension: ".html",
			Content:   `<h1>Nested <em>heading</em> text</h1>`,
			Title:     "Nested heading text",
		},
		{
			Name:      "no heading",
			Extension: ".html",
			Content:   `<h2>Heading</h2>`,
		},
		{
			Name:      "rendered Markdown heading",
			Extension: ".md",
			Content:   "# Markdown *heading*\n",
			Title:     "Markdown heading",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectSite(t, dir)
			layout := `<html><head><title>{{with .Page.ContentTitle}}{{.}} – {{end}}{{.Site.Name}}</title></head><body>{{.Page.Content}}</body></html>`
			if err := os.WriteFile(filepath.Join(dir, "src", "layouts", "default.html"), []byte(layout), 0644); err != nil {
				t.Fatal(err)
			}
			pageDir := filepath.Join(dir, "src", "pages")
			if tc.Extension != ".html" {
				if err := os.Remove(filepath.Join(pageDir, "index.html")); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(pageDir, "index"+tc.Extension), []byte(tc.Content), 0644); err != nil {
				t.Fatal(err)
			}

			if err := ssg.Generate(&ssg.GenerateOptions{Dir: dir, SiteName: "Test"}); err != nil {
				t.Fatal(err)
			}
			generated, err := os.ReadFile(filepath.Join(dir, "public", "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			want := "<title>Test</title>"
			if tc.Title != "" {
				want = "<title>" + tc.Title + " – Test</title>"
			}
			if got := string(generated); !strings.Contains(got, want) {
				t.Errorf("generated HTML: got: %q, want content containing: %q", got, want)
			}
		})
	}
}

func TestGenerateDoesNotSetMissingTitle(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "src", "pages", "index.html"), []byte(`<h1>Heading</h1>`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ssg.Generate(&ssg.GenerateOptions{Dir: dir, SiteName: "Test"}); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(filepath.Join(dir, "public", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(generated), "<title") {
		t.Errorf("generated HTML contains an implicit title: %q", generated)
	}
}
