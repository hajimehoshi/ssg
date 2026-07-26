// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package ssg_test

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hajimehoshi/ssg"
)

func TestGenerateSiteMetadata(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)

	layout := `<html><body>{{.Site.Name}}|{{.Site.URL}}|{{index .Site.Meta "title"}}|{{index (index .Site.Meta "author") "name"}}|{{index .Site.Meta "draft"}}|{{.Page.Content}}</body></html>`
	if err := os.WriteFile(filepath.Join(dir, "src", "layouts", "default.html"), []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}
	meta := "title: Site title\nauthor:\n  name: Hajime\ndraft: true\n"
	if err := os.WriteFile(filepath.Join(dir, "src", "meta.yaml"), []byte(meta), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ssg.Generate(&ssg.GenerateOptions{
		Dir:      dir,
		SiteName: "Test site",
		SiteURL:  "https://example.com",
	}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "public", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), "Test site|https://example.com|Site title|Hajime|true|<p>one</p>"; !strings.Contains(got, want) {
		t.Errorf("generated HTML: got: %q, want content containing: %q", got, want)
	}
}

func TestGenerateWithoutSiteMetadata(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)

	layout := `<html><body>{{if .Site.Meta}}unexpected metadata{{else}}empty metadata{{end}}</body></html>`
	if err := os.WriteFile(filepath.Join(dir, "src", "layouts", "default.html"), []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ssg.Generate(&ssg.GenerateOptions{
		Dir:      dir,
		SiteName: "Test",
	}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "public", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), "empty metadata"; !strings.Contains(got, want) {
		t.Errorf("generated HTML: got: %q, want content containing: %q", got, want)
	}
}

func TestGenerateRejectsInvalidSiteMetadata(t *testing.T) {
	testCases := []struct {
		Name string
		Meta string
	}{
		{
			Name: "malformed",
			Meta: "title: [",
		},
		{
			Name: "scalar",
			Meta: "site title",
		},
		{
			Name: "sequence",
			Meta: "- first\n- second\n",
		},
		{
			Name: "empty document",
			Meta: "",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectSite(t, dir)
			if err := os.WriteFile(filepath.Join(dir, "src", "meta.yaml"), []byte(tc.Meta), 0644); err != nil {
				t.Fatal(err)
			}

			err := ssg.Generate(&ssg.GenerateOptions{
				Dir:      dir,
				SiteName: "Test",
			})
			if err == nil {
				t.Error("Generate succeeded with invalid site metadata")
			}
		})
	}
}

func TestGenerateResourceVersionQuery(t *testing.T) {
	dir := t.TempDir()
	pageDir := filepath.Join(dir, "src", "pages")
	assetDir := filepath.Join(dir, "src", "assets")
	staticDir := filepath.Join(dir, "src", "static")
	layoutDir := filepath.Join(dir, "src", "layouts")
	outDir := filepath.Join(dir, "public")
	for _, path := range []string{pageDir, assetDir, staticDir, layoutDir} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(pageDir, "index.html"):   `<p>hello</p>`,
		filepath.Join(assetDir, "site.css"):    `body { color: red; }`,
		filepath.Join(staticDir, "unused.bin"): `unused`,
	} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
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

func TestGenerateCopiesStaticFilesUnchanged(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)

	const staticCSS = "/* Keep this comment. */\nbody { color: red; }\n"
	if err := os.WriteFile(filepath.Join(dir, "src", "static", "site.css"), []byte(staticCSS), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "static", "_headers"), []byte("headers"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ssg.Generate(&ssg.GenerateOptions{Dir: dir, SiteName: "Test"}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "public", "site.css"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content); got != staticCSS {
		t.Errorf("static file: got: %q, want: %q", got, staticCSS)
	}
	if content, err := os.ReadFile(filepath.Join(dir, "public", "_headers")); err != nil {
		t.Error(err)
	} else if got, want := string(content), "headers"; got != want {
		t.Errorf("underscored static file: got: %q, want: %q", got, want)
	}
}

func TestGenerateRejectsInvalidSourceTreeFile(t *testing.T) {
	testCases := []struct {
		Name string
		Path string
	}{
		{
			Name: "non-HTML page",
			Path: filepath.Join("src", "pages", "data.json"),
		},
		{
			Name: "unsupported asset",
			Path: filepath.Join("src", "assets", "image.png"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectSite(t, dir)
			if err := os.WriteFile(filepath.Join(dir, tc.Path), []byte("invalid"), 0644); err != nil {
				t.Fatal(err)
			}

			if err := ssg.Generate(&ssg.GenerateOptions{Dir: dir, SiteName: "Test"}); err == nil {
				t.Fatal("Generate succeeded with a file in the wrong source tree")
			}
		})
	}
}

func TestGenerateRejectsOutputPathCollision(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "src", "static", "index.html"), []byte("static"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ssg.Generate(&ssg.GenerateOptions{Dir: dir, SiteName: "Test"}); err == nil {
		t.Fatal("Generate succeeded with colliding page and static files")
	}
}

func TestGenerateRejectsMissingResource(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "src", "pages", "index.html"), []byte(`<img src="/missing.svg">`), 0644); err != nil {
		t.Fatal(err)
	}

	err := ssg.Generate(&ssg.GenerateOptions{
		Dir:      dir,
		SiteName: "Test",
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Generate: got: %v, want an error matching os.ErrNotExist", err)
	}
}

func TestGenerateMinifiesStyleElements(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)

	layout := `<html><head><style>
body {
  color: red;
}
</style></head><body>{{.Page.Content}}</body></html>`
	if err := os.WriteFile(filepath.Join(dir, "src", "layouts", "default.html"), []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}
	content := `<style>
p {
  display: block;
}
</style><p>one</p>`
	if err := os.WriteFile(filepath.Join(dir, "src", "pages", "index.html"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ssg.Generate(&ssg.GenerateOptions{
		Dir:      dir,
		SiteName: "Test",
	}); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(filepath.Join(dir, "public", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<style>body{color:red}</style>`,
		`<style>p{display:block}</style>`,
	} {
		if !strings.Contains(string(generated), want) {
			t.Errorf("generated HTML: got: %q, want content containing: %q", generated, want)
		}
	}
}

func TestGenerateRejectsInvalidInlineCSS(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)

	invalidCSS := "\\\n"
	layout := `<html><head><style>` + invalidCSS + `</style></head><body>{{.Page.Content}}</body></html>`
	if err := os.WriteFile(filepath.Join(dir, "src", "layouts", "default.html"), []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}

	err := ssg.Generate(&ssg.GenerateOptions{
		Dir:      dir,
		SiteName: "Test",
	})
	if err == nil {
		t.Fatal("Generate succeeded with invalid inline CSS")
	}
}

func TestGenerateMinifiesScriptElements(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)

	layout := `<html><head><script>
// Mark the document as ready.
window.ready = 1;
</script><script type="">
window.emptyType = 2;
</script><script type="module">
export default 3;
</script><script type="Application/JavaScript">
window.applicationType = 4;
</script><script type="text/ecmascript">
window.ecmascriptType = 5;
</script><script type="application/json">
{ "answer": 42 }
</script><script type="text/javascript; charset=utf-8">
window.parameterizedType = 6;
</script></head><body>{{.Page.Content}}</body></html>`
	if err := os.WriteFile(filepath.Join(dir, "src", "layouts", "default.html"), []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}
	content := `<script type="text/javascript">
window.loaded = 1;
</script><p>one</p>`
	if err := os.WriteFile(filepath.Join(dir, "src", "pages", "index.html"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ssg.Generate(&ssg.GenerateOptions{
		Dir:      dir,
		SiteName: "Test",
	}); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(filepath.Join(dir, "public", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<script>window.ready=1;</script>`,
		`<script type="">window.emptyType=2;</script>`,
		`<script type="module">export default 3;</script>`,
		`<script type="Application/JavaScript">window.applicationType=4;</script>`,
		`<script type="text/ecmascript">window.ecmascriptType=5;</script>`,
		`<script type="text/javascript">window.loaded=1;</script>`,
		"<script type=\"application/json\">\n{ \"answer\": 42 }\n</script>",
		"<script type=\"text/javascript; charset=utf-8\">\nwindow.parameterizedType = 6;\n</script>",
	} {
		if !strings.Contains(string(generated), want) {
			t.Errorf("generated HTML: got: %q, want content containing: %q", generated, want)
		}
	}
}

func TestGenerateRejectsInvalidInlineJavaScript(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)

	layout := `<html><head><script>const = ;</script></head><body>{{.Page.Content}}</body></html>`
	if err := os.WriteFile(filepath.Join(dir, "src", "layouts", "default.html"), []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}

	err := ssg.Generate(&ssg.GenerateOptions{
		Dir:      dir,
		SiteName: "Test",
	})
	if err == nil {
		t.Fatal("Generate succeeded with invalid inline JavaScript")
	}
}

func TestGenerateSelectsLayout(t *testing.T) {
	dir := t.TempDir()
	pageDir := filepath.Join(dir, "src", "pages")
	layoutDir := filepath.Join(dir, "src", "layouts")
	outputDir := filepath.Join(dir, "public")
	for _, path := range []string{
		filepath.Join(pageDir, "writings"),
		filepath.Join(layoutDir, "writings"),
	} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(pageDir, "index.html"): `<p>home</p>`,
		filepath.Join(pageDir, "normalized.html"): `<script type="application/yaml">
_layout: writings/../default
</script><p>normalized</p>`,
		filepath.Join(pageDir, "writings", "index.html"): `<script type="application/yaml">
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
		OutsidePath string
	}{
		{
			Name:   "not a string",
			Layout: "3",
		},
		{
			Name:   "empty",
			Layout: `""`,
		},
		{
			Name:        "parent traversal",
			Layout:      "../article",
			OutsidePath: filepath.Join("src", "article.html"),
		},
		{
			Name:   "parent traversal back into layouts",
			Layout: "../layouts/default",
		},
		{
			Name:   "absolute",
			Layout: "/article",
		},
		{
			Name:   "backslash",
			Layout: `"blog\\article"`,
		},
		{
			Name:   "missing",
			Layout: "missing",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			dir := t.TempDir()
			pageDir := filepath.Join(dir, "src", "pages")
			layoutDir := filepath.Join(dir, "src", "layouts")
			for _, path := range []string{pageDir, layoutDir} {
				if err := os.MkdirAll(path, 0755); err != nil {
					t.Fatal(err)
				}
			}
			content := `<script type="application/yaml">_layout: ` + tc.Layout + `</script><p>hello</p>`
			if err := os.WriteFile(filepath.Join(pageDir, "index.html"), []byte(content), 0644); err != nil {
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
			if err == nil {
				t.Error("Generate succeeded with an invalid layout")
			}
		})
	}
}

func TestGenerateAllowsLayoutSymlinkOutsideLayoutDir(t *testing.T) {
	dir := t.TempDir()
	pageDir := filepath.Join(dir, "src", "pages")
	layoutDir := filepath.Join(dir, "src", "layouts")
	for _, path := range []string{pageDir, layoutDir} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	content := `<script type="application/yaml">_layout: external</script><p>hello</p>`
	if err := os.WriteFile(filepath.Join(pageDir, "index.html"), []byte(content), 0644); err != nil {
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

	if err := ssg.Generate(&ssg.GenerateOptions{
		Dir:      dir,
		SiteName: "Test",
	}); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(filepath.Join(dir, "public", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "<p>hello</p>") {
		t.Errorf("generated page does not contain the page content: %q", generated)
	}
}
