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

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"

	"github.com/hajimehoshi/ssg/internal/htmlrewrite"
	"github.com/hajimehoshi/ssg/internal/pagepath"
)

var markdownConverter = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(goldmarkhtml.WithUnsafe()),
)

func generatePages(outDir, pageDir, layoutDir string, options *GenerateOptions, mode generationMode) error {
	// templates maps each normalized layout path to its parsed template. Building
	// it before concurrent generation lets the goroutines read it without
	// locking.
	templates := map[string]*template.Template{}
	var images imageCache
	sharedTemplates := template.New("").Funcs(template.FuncMap{
		"imageMetadata": func(path string) (*imageData, error) {
			return images.get(outDir, path, mode)
		},
	})
	var layoutPaths []string
	if err := filepath.Walk(layoutDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		isSharedTemplate := strings.HasPrefix(filepath.Base(path), "_")
		if isIgnoredFile(path) && !isSharedTemplate {
			return nil
		}
		if !isSharedTemplate {
			layoutPaths = append(layoutPaths, path)
			return nil
		}
		rel, err := filepath.Rel(layoutDir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := sharedTemplates.New(filepath.ToSlash(rel)).Parse(string(data)); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	for _, path := range layoutPaths {
		rel, err := filepath.Rel(layoutDir, path)
		if err != nil {
			return err
		}
		namePath := strings.TrimSuffix(rel, ".html")
		resolvedPath, err := resolveLayoutPath(layoutDir, namePath)
		if errors.Is(err, errLayoutOutsideDir) || errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		data, err := os.ReadFile(resolvedPath)
		if err != nil {
			return err
		}
		tmpl, err := sharedTemplates.Clone()
		if err != nil {
			return err
		}
		tmpl, err = tmpl.New(filepath.Base(path)).Parse(string(data))
		if err != nil {
			return err
		}
		templates[resolvedPath] = tmpl
	}

	directoryMetadata := map[string]map[string]any{}
	var pagePaths []string
	if err := filepath.Walk(pageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) == directoryMetadataFilename {
			meta, err := loadDirectoryMetadata(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(pageDir, filepath.Dir(path))
			if err != nil {
				return err
			}
			directoryMetadata[rel] = meta
			return nil
		}
		if !isPageExtension(filepath.Ext(path)) {
			return nil
		}
		if isIgnoredFile(path) {
			return nil
		}
		rel, err := filepath.Rel(pageDir, path)
		if err != nil {
			return err
		}
		pagePaths = append(pagePaths, rel)
		return nil
	}); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	var wg errgroup.Group
	for _, path := range pagePaths {
		wg.Go(func() error {
			return generatePage(path, directoryMetadata, templates, outDir, pageDir, layoutDir, options, mode)
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

// pagePath returns the site-root-absolute path of the generated page file at
// relPath. A trailing index.html is dropped so
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

func generatePage(sourcePath string, directoryMetadata map[string]map[string]any, templates map[string]*template.Template, outDir, pageDir, layoutDir string, options *GenerateOptions, mode generationMode) error {
	inPath := filepath.Join(pageDir, sourcePath)
	outputPath := pagepath.Output(sourcePath)
	outPath := filepath.Join(outDir, outputPath)

	content, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}

	var meta map[string]any
	switch filepath.Ext(sourcePath) {
	case ".html":
		meta, content, err = extractMetadataFromHTML(content)
	case ".md":
		meta, content, err = extractMetadataFromMarkdown(content)
	default:
		panic("unreachable")
	}
	if err != nil {
		return fmt.Errorf("ssg: extracting metadata in %s failed: %w", inPath, err)
	}
	meta = mergePageMetadata(directoryMetadata, sourcePath, meta)
	if filepath.Ext(sourcePath) == ".md" {
		var converted bytes.Buffer
		if err := markdownConverter.Convert(content, &converted); err != nil {
			return fmt.Errorf("ssg: converting Markdown in %s failed: %w", inPath, err)
		}
		content = converted.Bytes()
	}
	layoutPath, err := consumeLayoutPath(meta, sourcePath, layoutDir)
	if err != nil {
		return err
	}
	tmpl, ok := templates[layoutPath]
	if !ok {
		return fmt.Errorf("ssg: layout for %s not found", sourcePath)
	}

	urlPath := pagePath(outputPath, options.KeepHTMLExtension)

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

	missingResourceMode := htmlrewrite.ErrorOnMissingResource
	if mode == generationModeServe {
		missingResourceMode = htmlrewrite.IgnoreMissingResource
	}
	if err := htmlrewrite.AddFontPreloads(node, outDir, filepath.Dir(outputPath), missingResourceMode); err != nil {
		return err
	}

	if err := htmlrewrite.AddResourceVersions(node, outDir, filepath.Dir(outputPath), missingResourceMode); err != nil {
		return err
	}

	htmlrewrite.RewritePageLinks(node, options.KeepHTMLExtension)

	if err := htmlrewrite.Minify(node); err != nil {
		return err
	}

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
	if strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("ssg: layout path %q for %s must not start with ../", name, contentPath)
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
	if !strings.HasPrefix(candidate, withTrailingSeparator(absLayoutDir)) {
		return "", errLayoutOutsideDir
	}
	return candidate, nil
}

func withTrailingSeparator(path string) string {
	separator := string(filepath.Separator)
	if strings.HasSuffix(path, separator) {
		return path
	}
	return path + separator
}
