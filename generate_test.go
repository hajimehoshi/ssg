// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package ssg_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/hajimehoshi/ssg"
)

func TestGenerateVersionedResourceFilename(t *testing.T) {
	dir := t.TempDir()
	inDir := filepath.Join(dir, "content")
	outDir := filepath.Join(dir, "public")
	if err := os.MkdirAll(inDir, 0755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		"_tmpl.html": `<html><head><link rel="stylesheet" href="/site.css"/></head><body>{{.Page.Content}}</body></html>`,
		"index.html": `<p>hello</p>`,
		"site.css":   `body { color: red; }`,
		"unused.bin": `unused`,
	} {
		if err := os.WriteFile(filepath.Join(inDir, path), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := ssg.Generate(&ssg.GenerateOptions{
		InputDir:  inDir,
		OutputDir: outDir,
		SiteName:  "Test",
	}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`href="/(site\.[A-Za-z0-9_-]{10}\.css)"`).FindSubmatch(content)
	if match == nil {
		t.Fatalf("generated HTML has no versioned stylesheet: %q", content)
	}
	versioned, err := os.ReadFile(filepath.Join(outDir, string(match[1])))
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(outDir, "site.css"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(versioned), string(original); got != want {
		t.Errorf("versioned asset: got: %q, want: %q", got, want)
	}
	matches, err := filepath.Glob(filepath.Join(outDir, "unused.*.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("unreferenced asset has versioned copies: %q", matches)
	}
	if _, err := os.Stat(filepath.Join(outDir, "unused.bin")); err != nil {
		t.Errorf("unreferenced asset: %v", err)
	}
}
