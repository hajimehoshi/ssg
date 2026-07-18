// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023 Hajime Hoshi

package htmlrewrite_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hajimehoshi/ssg/internal/htmlrewrite"
)

func TestAddResourceVersionsLinkRel(t *testing.T) {
	outDir := t.TempDir()
	for _, f := range []struct {
		path    string
		content string
	}{
		{"style.css", "body { color: red }\n"},
		{"favicon.webp", "dummy\n"},
		{"startup.png", "dummy\n"},
		{"index.html", "<html></html>\n"},
	} {
		if err := os.WriteFile(filepath.Join(outDir, f.path), []byte(f.content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	testCases := []struct {
		Name      string
		In        string
		Versioned bool
	}{
		{
			Name:      "stylesheet",
			In:        `<link rel="stylesheet" href="/style.css"/>`,
			Versioned: true,
		},
		{
			Name:      "icon",
			In:        `<link rel="icon" href="/favicon.webp" type="image/webp"/>`,
			Versioned: true,
		},
		{
			Name:      "shortcut icon",
			In:        `<link rel="Shortcut Icon" href="/favicon.webp"/>`,
			Versioned: true,
		},
		{
			Name:      "apple-touch-startup-image",
			In:        `<link rel="apple-touch-startup-image" href="/startup.png"/>`,
			Versioned: true,
		},
		{
			// A prefetch target is a page, which may not be written yet.
			Name:      "prefetch",
			In:        `<link rel="prefetch" href="/ja/index.html"/>`,
			Versioned: false,
		},
		{
			// An origin is not a local file.
			Name:      "preconnect",
			In:        `<link rel="preconnect" href="https://res.example.com"/>`,
			Versioned: false,
		},
		{
			// A page rel points at another page's output file, which the
			// concurrent generation may not have written yet.
			Name:      "canonical",
			In:        `<link rel="canonical" href="/index.html"/>`,
			Versioned: false,
		},
		{
			Name:      "alternate to a page that is not generated yet",
			In:        `<link rel="alternate" hreflang="ja" href="/ja/index.html"/>`,
			Versioned: false,
		},
		{
			// The href resolves to the output directory itself.
			Name:      "alternate to the site root",
			In:        `<link rel="alternate" href="/"/>`,
			Versioned: false,
		},
		{
			Name:      "no rel",
			In:        `<link href="/index.html"/>`,
			Versioned: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			node := parseLinkElement(t, tc.In)
			versions, err := htmlrewrite.AddResourceVersions(node, outDir, ".")
			if err != nil {
				t.Fatal(err)
			}
			got := renderNode(t, node)
			if tc.Versioned {
				if got, want := len(versions), 1; got != want {
					t.Errorf("resource versions: got: %d, want: %d", got, want)
				}
				match := regexp.MustCompile(`/([^/]+\.[A-Za-z0-9_-]{10}\.[^/"?]+)`).FindStringSubmatch(got)
				if match == nil {
					t.Fatalf("got: %q, want a content hash in the filename", got)
				}
				if strings.Contains(got, "?v=") {
					t.Errorf("got: %q, want no cache-busting query", got)
				}
				return
			}
			if got := len(versions); got != 0 {
				t.Errorf("resource versions: got: %d, want: 0", got)
			}
			if want := tc.In; got != want {
				t.Errorf("got: %q, want: %q", got, want)
			}
		})
	}
}

func TestAddResourceVersionsPreservesQueryAndFragment(t *testing.T) {
	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "site.css"), []byte("body{}"), 0644); err != nil {
		t.Fatal(err)
	}

	node := parseLinkElement(t, `<link rel="stylesheet" href="/site.css?theme=dark#top"/>`)
	if _, err := htmlrewrite.AddResourceVersions(node, outDir, "."); err != nil {
		t.Fatal(err)
	}
	got := renderNode(t, node)
	if !regexp.MustCompile(`href="/site\.[A-Za-z0-9_-]{10}\.css\?theme=dark#top"`).MatchString(got) {
		t.Errorf("got: %q, want a versioned filename with the query and fragment preserved", got)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(entries), 1; got != want {
		t.Errorf("output files: got: %d, want: %d", got, want)
	}
}

func TestAddResourceVersionsRefreshesHash(t *testing.T) {
	outDir := t.TempDir()
	path := filepath.Join(outDir, "site.css")
	var urls []string
	for _, content := range []string{"body{color:red}", "body{color:blue}"} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		node := parseLinkElement(t, `<link rel="stylesheet" href="/site.css"/>`)
		if _, err := htmlrewrite.AddResourceVersions(node, outDir, "."); err != nil {
			t.Fatal(err)
		}
		urls = append(urls, renderNode(t, node))
	}
	if urls[0] == urls[1] {
		t.Errorf("resource URL did not change after its content changed: %q", urls[0])
	}
}
