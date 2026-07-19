// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023 Hajime Hoshi

package ssg

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"

	"github.com/hajimehoshi/ssg/internal/htmlrewrite"
)

func generateHTMLs(outDir, inDir, layoutDir string, options *GenerateOptions) error {
	// templates maps each resolved layout path to its parsed template. Building
	// it before concurrent generation lets the goroutines read it without
	// locking.
	templates := map[string]*template.Template{}
	if err := filepath.Walk(layoutDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		rel, err := filepath.Rel(layoutDir, path)
		if err != nil {
			return err
		}
		namePath := strings.TrimSuffix(rel, ".html")
		resolvedPath, err := resolveLayoutPath(layoutDir, namePath)
		if errors.Is(err, errLayoutOutsideDir) || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		data, err := os.ReadFile(resolvedPath)
		if err != nil {
			return err
		}
		tmpl, err := template.New(filepath.Base(path)).Parse(string(data))
		if err != nil {
			return err
		}
		templates[resolvedPath] = tmpl
		return nil
	}); err != nil {
		return err
	}

	var contentPaths []string
	if err := filepath.Walk(inDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		if isIgnoredFile(path) {
			return nil
		}
		rel, err := filepath.Rel(inDir, path)
		if err != nil {
			return err
		}
		contentPaths = append(contentPaths, rel)
		return nil
	}); err != nil {
		return err
	}

	var wg errgroup.Group
	for _, path := range contentPaths {
		wg.Go(func() error {
			return generateHTML(path, templates, outDir, inDir, layoutDir, options)
		})
	}
	return wg.Wait()
}

// siteData is the site-wide data available to templates as .Site.
type siteData struct {
	Name string
	URL  string
}

// pageData is the per-page data available to templates as .Page.
type pageData struct {
	Path    string
	URL     string
	Meta    map[string]any
	Content template.HTML
}

// pagePath returns the site-root-absolute path of the content file at relPath,
// which is relative to the content root. A trailing index.html is dropped so
// that the path denotes the directory the browser requests; any other .html
// extension is dropped unless keepHTMLExtension is set.
func pagePath(relPath string, keepHTMLExtension bool) string {
	p := "/" + filepath.ToSlash(relPath)
	if strings.HasSuffix(p, "/index.html") {
		return strings.TrimSuffix(p, "index.html")
	}
	if !keepHTMLExtension {
		p = strings.TrimSuffix(p, ".html")
	}
	return p
}

// pageURL returns the absolute URL of the page at the site-root-absolute path,
// or an empty string when siteURL is empty.
func pageURL(siteURL, path string) string {
	if siteURL == "" {
		return ""
	}
	return strings.TrimSuffix(siteURL, "/") + path
}

func generateHTML(path string, templates map[string]*template.Template, outDir, inDir, layoutDir string, options *GenerateOptions) error {
	inPath := filepath.Join(inDir, path)
	outPath := filepath.Join(outDir, path)

	content, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}

	meta, content, err := extractMetadataFromHTML(content)
	if err != nil {
		return fmt.Errorf("ssg: extracting metadata in %s failed: %w", inPath, err)
	}
	layoutPath, err := consumeLayoutPath(meta, path, layoutDir)
	if err != nil {
		return err
	}
	tmpl, ok := templates[layoutPath]
	if !ok {
		return fmt.Errorf("ssg: layout for %s not found", path)
	}

	urlPath := pagePath(path, options.KeepHTMLExtension)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct {
		Site siteData
		Page pageData
	}{
		Site: siteData{
			Name: options.SiteName,
			URL:  options.SiteURL,
		},
		Page: pageData{
			Path:    urlPath,
			URL:     pageURL(options.SiteURL, urlPath),
			Meta:    meta,
			Content: template.HTML(content),
		},
	}); err != nil {
		return err
	}

	node, err := html.Parse(&buf)
	if err != nil {
		return err
	}

	htmlrewrite.SetMissingTitle(node, options.SiteName)

	if err := htmlrewrite.AddFontPreloads(node, outDir, filepath.Dir(path)); err != nil {
		return err
	}

	if err := htmlrewrite.AddResourceVersions(node, outDir, filepath.Dir(path)); err != nil {
		return err
	}

	htmlrewrite.RewritePageLinks(node, options.KeepHTMLExtension)

	htmlrewrite.Minify(node)

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	w := bufio.NewWriter(out)
	if err := html.Render(w, node); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return err
	}

	return nil
}

func consumeLayoutPath(meta map[string]any, contentPath, layoutDir string) (string, error) {
	const defaultLayout = "default"

	value, ok := meta["_layout"]
	if !ok {
		value = defaultLayout
	} else {
		delete(meta, "_layout")
	}
	name, ok := value.(string)
	if !ok || name == "" {
		return "", fmt.Errorf("ssg: _layout for %s must be a non-empty string", contentPath)
	}
	if strings.Contains(name, `\`) {
		return "", fmt.Errorf("ssg: layout path %q for %s must use forward slashes", name, contentPath)
	}
	namePath := filepath.FromSlash(name)
	if filepath.IsAbs(namePath) {
		return "", fmt.Errorf("ssg: layout path %q for %s must be relative", name, contentPath)
	}
	resolvedPath, err := resolveLayoutPath(layoutDir, namePath)
	if errors.Is(err, errLayoutOutsideDir) {
		return "", fmt.Errorf("ssg: layout path %q for %s is outside the layouts directory", name, contentPath)
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("ssg: layout %q for %s not found", name, contentPath)
	}
	if err != nil {
		return "", err
	}
	return resolvedPath, nil
}

var errLayoutOutsideDir = errors.New("ssg: layout path is outside the layouts directory")

func resolveLayoutPath(layoutDir, namePath string) (string, error) {
	absLayoutDir, err := filepath.Abs(layoutDir)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Abs(filepath.Join(layoutDir, namePath) + ".html")
	if err != nil {
		return "", err
	}
	resolvedLayoutDir, err := filepath.EvalSymlinks(absLayoutDir)
	if err != nil {
		return "", err
	}
	resolvedCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(resolvedCandidate, withTrailingSeparator(resolvedLayoutDir)) {
		return "", errLayoutOutsideDir
	}
	return resolvedCandidate, nil
}

func withTrailingSeparator(path string) string {
	separator := string(filepath.Separator)
	if strings.HasSuffix(path, separator) {
		return path
	}
	return path + separator
}
