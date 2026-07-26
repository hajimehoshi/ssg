// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

// Package ssg generates a static website from page, asset, static, and layout
// files.
package ssg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GenerateOptions is options for Generate.
type GenerateOptions struct {
	// Dir is the project directory containing src and public. The default is
	// the current directory.
	Dir string

	// SiteName is the name of the website, used e.g. in page titles.
	SiteName string

	// SiteURL is the absolute URL of the website root, used when a page
	// needs an absolute URL. This can be empty.
	SiteURL string

	// KeepHTMLExtension keeps the .html extension in page URLs. By default a
	// page URL omits it, which requires the server to resolve an extensionless
	// URL to its .html file.
	KeepHTMLExtension bool
}

func Generate(options *GenerateOptions) error {
	return generate(options, generationModeStrict)
}

type generationMode int

const (
	generationModeStrict generationMode = iota
	generationModeServe
)

func generate(options *GenerateOptions, mode generationMode) error {
	if options == nil || options.SiteName == "" {
		return fmt.Errorf("ssg: SiteName must not be empty")
	}

	meta, err := loadSiteMetadata(options.metadataPath())
	if err != nil {
		return err
	}

	pageDir := options.pageDir()
	assetDir := options.assetDir()
	staticDir := options.staticDir()
	layoutDir := options.layoutDir()
	outputDir := options.outputDir()
	if err := validateSourceTrees(pageDir, assetDir, staticDir); err != nil {
		return err
	}
	if err := os.RemoveAll(outputDir); err != nil {
		return err
	}
	if err := copyStaticFiles(outputDir, staticDir); err != nil {
		return err
	}
	if err := generateAssets(outputDir, assetDir); err != nil {
		return err
	}
	if err := generateHTMLs(outputDir, pageDir, layoutDir, meta, options, mode); err != nil {
		return err
	}
	return nil
}

func (o *GenerateOptions) sourceDir() string {
	return filepath.Join(o.Dir, "src")
}

func (o *GenerateOptions) pageDir() string {
	return filepath.Join(o.sourceDir(), "pages")
}

func (o *GenerateOptions) assetDir() string {
	return filepath.Join(o.sourceDir(), "assets")
}

func (o *GenerateOptions) staticDir() string {
	return filepath.Join(o.sourceDir(), "static")
}

func (o *GenerateOptions) layoutDir() string {
	return filepath.Join(o.sourceDir(), "layouts")
}

func (o *GenerateOptions) metadataPath() string {
	return filepath.Join(o.sourceDir(), "meta.yaml")
}

func (o *GenerateOptions) outputDir() string {
	return filepath.Join(o.Dir, "public")
}

func isIgnoredFile(path string) bool {
	if strings.HasPrefix(filepath.Base(path), "#") {
		return true
	}
	if strings.HasPrefix(filepath.Base(path), "_") {
		return true
	}
	if strings.HasSuffix(path, "~") {
		return true
	}
	return false
}
