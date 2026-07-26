// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package ssg_test

import (
	"errors"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hajimehoshi/ssg"
)

func TestGenerateInspectsLocalImages(t *testing.T) {
	testCases := []struct {
		Name      string
		Path      string
		MediaType string
		Width     int
		Height    int
		Encode    func(*os.File, image.Image) error
	}{
		{
			Name:      "GIF",
			Path:      "/images/test.gif",
			MediaType: "image/gif",
			Width:     13,
			Height:    17,
			Encode: func(file *os.File, img image.Image) error {
				return gif.Encode(file, img, nil)
			},
		},
		{
			Name:      "JPEG",
			Path:      "/images/test.jpg",
			MediaType: "image/jpeg",
			Width:     13,
			Height:    17,
			Encode: func(file *os.File, img image.Image) error {
				return jpeg.Encode(file, img, nil)
			},
		},
		{
			Name:      "PNG",
			Path:      "/images/test.png",
			MediaType: "image/png",
			Width:     13,
			Height:    17,
			Encode: func(file *os.File, img image.Image) error {
				return png.Encode(file, img)
			},
		},
		{
			Name:      "WebP",
			Path:      "/images/test.webp",
			MediaType: "image/webp",
			Width:     75,
			Height:    100,
			Encode: func(file *os.File, _ image.Image) error {
				_, err := file.Write([]byte(webPConfig))
				return err
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectSite(t, dir)
			imagePath := filepath.Join(dir, "src", "static", filepath.FromSlash(strings.TrimPrefix(tc.Path, "/")))
			if err := os.MkdirAll(filepath.Dir(imagePath), 0755); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(imagePath)
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.Encode(file, image.NewNRGBA(image.Rect(0, 0, 13, 17))); err != nil {
				file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}

			layout := `<html><body>{{with imageMetadata ` + strconv.Quote(tc.Path) + `}}{{.MediaType}}|{{.Width}}|{{.Height}}{{end}}</body></html>`
			if err := os.WriteFile(filepath.Join(dir, "src", "layouts", "default.html"), []byte(layout), 0644); err != nil {
				t.Fatal(err)
			}
			if err := ssg.Generate(&ssg.GenerateOptions{Dir: dir, SiteName: "Test"}); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(filepath.Join(dir, "public", "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			want := tc.MediaType + "|" + strconv.Itoa(tc.Width) + "|" + strconv.Itoa(tc.Height)
			if got := string(content); !strings.Contains(got, want) {
				t.Errorf("generated HTML: got: %q, want content containing: %q", got, want)
			}
		})
	}
}

func TestImageCache(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "image.png")
	writePNG := func(width, height int) {
		t.Helper()

		file, err := os.Create(imagePath)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, image.NewNRGBA(image.Rect(0, 0, width, height))); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}

	writePNG(13, 17)
	var cache ssg.ImageCache
	first, err := cache.Get(dir, "/image.png")
	if err != nil {
		t.Fatal(err)
	}

	writePNG(19, 23)
	second, err := cache.Get(dir, "/image.png")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := second, first; got != want {
		t.Errorf("second inspection: got: %v, want cached result: %v", got, want)
	}
	if got, want := second.Width, 13; got != want {
		t.Errorf("second inspection width: got: %d, want: %d", got, want)
	}
	if got, want := second.Height, 17; got != want {
		t.Errorf("second inspection height: got: %d, want: %d", got, want)
	}
}

// webPConfig contains a 75x100 extended WebP header without image data. It is
// sufficient for DecodeConfig and would fail if the full image were decoded.
const webPConfig = "RIFF\x16\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00\x00\x00\x00\x00\x4a\x00\x00\x63\x00\x00"

func TestGenerateRejectsInvalidLocalImage(t *testing.T) {
	testCases := []struct {
		Name       string
		ImagePath  string
		CreatePath string
		Content    string
		Directory  bool
	}{
		{
			Name:      "missing",
			ImagePath: "/missing.png",
		},
		{
			Name:       "unsupported format",
			ImagePath:  "/image.bmp",
			CreatePath: "image.bmp",
			Content:    "not a registered image format",
		},
		{
			Name:       "directory",
			ImagePath:  "/images",
			CreatePath: "images",
			Directory:  true,
		},
		{
			Name:      "relative path",
			ImagePath: "images/test.png",
		},
		{
			Name:      "parent traversal",
			ImagePath: "/images/../test.png",
		},
		{
			Name:      "backslash",
			ImagePath: `/images\test.png`,
		},
		{
			Name:      "invalid escape",
			ImagePath: "/images/%test.png",
		},
		{
			Name:      "query",
			ImagePath: "/image.png?v=1",
		},
		{
			Name:      "fragment",
			ImagePath: "/image.png#thumbnail",
		},
		{
			Name:      "empty fragment",
			ImagePath: "/image.png#",
		},
		{
			Name:      "remote URL",
			ImagePath: "https://example.com/image.png",
		},
		{
			Name:      "protocol-relative URL",
			ImagePath: "//example.com/image.png",
		},
		{
			Name:      "encoded protocol-relative path",
			ImagePath: "/%2Fexample.com/image.png",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectSite(t, dir)
			if tc.CreatePath != "" {
				resourcePath := filepath.Join(dir, "src", "static", filepath.FromSlash(tc.CreatePath))
				var err error
				if tc.Directory {
					err = os.MkdirAll(resourcePath, 0755)
				} else {
					err = os.WriteFile(resourcePath, []byte(tc.Content), 0644)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			writeImageLayout(t, dir, tc.ImagePath)

			err := ssg.Generate(&ssg.GenerateOptions{Dir: dir, SiteName: "Test"})
			if err == nil {
				t.Fatal("Generate succeeded")
			}
			if tc.Name == "missing" && !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Generate: got: %v, want an error matching os.ErrNotExist", err)
			}
		})
	}
}

func TestGenerateInspectsLocalImageSymlink(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)
	outsidePath := filepath.Join(dir, "outside.png")
	file, err := os.Create(outsidePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(dir, "src", "static", "image.png")); err != nil {
		t.Fatal(err)
	}
	layout := `<html><body>{{with imageMetadata "/image.png"}}{{.MediaType}}|{{.Width}}|{{.Height}}{{end}}</body></html>`
	if err := os.WriteFile(filepath.Join(dir, "src", "layouts", "default.html"), []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ssg.Generate(&ssg.GenerateOptions{Dir: dir, SiteName: "Test"}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "public", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), "image/png|1|1"; !strings.Contains(got, want) {
		t.Errorf("generated HTML: got: %q, want content containing: %q", got, want)
	}
	info, err := os.Lstat(filepath.Join(dir, "public", "image.png"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("deployed image is a symbolic link, want a regular copied file")
	}
}

func TestGenerateForServeIgnoresMissingLocalImage(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)
	layout := `<html><body>{{with imageMetadata "/missing.png"}}unexpected image{{else}}image unavailable{{end}}</body></html>`
	if err := os.WriteFile(filepath.Join(dir, "src", "layouts", "default.html"), []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ssg.GenerateForServe(&ssg.GenerateOptions{Dir: dir, SiteName: "Test"}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "public", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), "image unavailable"; !strings.Contains(got, want) {
		t.Errorf("generated HTML: got: %q, want content containing: %q", got, want)
	}
}

func writeImageLayout(t *testing.T, dir, imagePath string) {
	t.Helper()

	layout := `<html><body>{{with imageMetadata ` + strconv.Quote(imagePath) + `}}{{.MediaType}}{{end}}</body></html>`
	if err := os.WriteFile(filepath.Join(dir, "src", "layouts", "default.html"), []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}
}
