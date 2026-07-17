// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

// Package ssg generates a static website from a directory of contents.
package ssg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultInputDir  = "content"
	defaultOutputDir = "public"
)

// GenerateOptions is options for Generate.
type GenerateOptions struct {
	// InputDir is the directory containing the source files. The default is
	// "content".
	InputDir string

	// OutputDir is the directory to generate the site into. The default is
	// "public".
	OutputDir string

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
	if options == nil || options.SiteName == "" {
		return fmt.Errorf("ssg: SiteName must not be empty")
	}

	inputDir := options.inputDir()
	outputDir := options.outputDir()
	if err := os.RemoveAll(outputDir); err != nil {
		return err
	}
	if err := copyNonHTMLFiles(outputDir, inputDir); err != nil {
		return err
	}
	if err := generateHTMLs(outputDir, inputDir, options); err != nil {
		return err
	}
	return nil
}

func (o *GenerateOptions) inputDir() string {
	if o.InputDir != "" {
		return o.InputDir
	}
	return defaultInputDir
}

func (o *GenerateOptions) outputDir() string {
	if o.OutputDir != "" {
		return o.OutputDir
	}
	return defaultOutputDir
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
