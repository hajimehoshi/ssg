// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package ssg

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp"
)

// imageData is metadata about a local image available to templates.
type imageData struct {
	MediaType string
	Width     int
	Height    int
}

func inspectImage(resourceRoot, resourcePath string, mode generationMode) (*imageData, error) {
	filePath, err := resolveResourcePath(resourceRoot, resourcePath)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(filePath)
	if mode == generationModeServe && errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ssg: opening image %q failed: %w", resourcePath, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("ssg: inspecting image %q failed: %w", resourcePath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("ssg: image path %q is a directory", resourcePath)
	}

	config, format, err := image.DecodeConfig(file)
	if err != nil {
		return nil, fmt.Errorf("ssg: decoding image config for %q failed: %w", resourcePath, err)
	}
	mediaType, err := imageMediaType(format)
	if err != nil {
		return nil, fmt.Errorf("ssg: decoding image config for %q failed: %w", resourcePath, err)
	}
	return &imageData{
		MediaType: mediaType,
		Width:     config.Width,
		Height:    config.Height,
	}, nil
}

func resolveResourcePath(resourceRoot, resourcePath string) (string, error) {
	if strings.Contains(resourcePath, `\`) {
		return "", fmt.Errorf("ssg: image path %q must use forward slashes", resourcePath)
	}
	u, err := url.Parse(resourcePath)
	if err != nil {
		return "", fmt.Errorf("ssg: image path %q is malformed: %w", resourcePath, err)
	}
	if resourcePath == "" || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return "", fmt.Errorf("ssg: image path %q must be a site-root-relative local path", resourcePath)
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(resourcePath, "#") {
		return "", fmt.Errorf("ssg: image path %q must not contain a query or fragment", resourcePath)
	}
	if strings.ContainsRune(u.Path, '\x00') || path.Clean(u.Path) != u.Path {
		return "", fmt.Errorf("ssg: image path %q is malformed", resourcePath)
	}

	relPath := filepath.FromSlash(strings.TrimPrefix(u.Path, "/"))
	if relPath == "" {
		relPath = "."
	}
	if !filepath.IsLocal(relPath) {
		return "", fmt.Errorf("ssg: image path %q is outside the site resource root", resourcePath)
	}
	return filepath.Join(resourceRoot, relPath), nil
}

func imageMediaType(format string) (string, error) {
	switch format {
	case "gif":
		return "image/gif", nil
	case "jpeg":
		return "image/jpeg", nil
	case "png":
		return "image/png", nil
	case "webp":
		return "image/webp", nil
	default:
		return "", fmt.Errorf("unsupported image format %q", format)
	}
}
