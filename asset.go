// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023 Hajime Hoshi

package ssg

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/hajimehoshi/ssg/internal/htmlrewrite"
)

type sourceTree struct {
	dir              string
	kind             string
	applyIgnoreRules bool
}

func validateSourceTrees(pageDir, assetDir, staticDir string) error {
	trees := []sourceTree{
		{dir: pageDir, kind: "page", applyIgnoreRules: true},
		{dir: assetDir, kind: "asset", applyIgnoreRules: true},
		{dir: staticDir, kind: "static"},
	}
	outputs := map[string]sourceTree{}
	for _, tree := range trees {
		err := filepath.Walk(tree.dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || tree.applyIgnoreRules && isIgnoredFile(path) {
				return nil
			}
			ext := filepath.Ext(path)
			switch tree.kind {
			case "page":
				if ext != ".html" {
					return fmt.Errorf("ssg: page file %s must have an .html extension", path)
				}
			case "asset":
				if ext != ".css" && ext != ".js" {
					return fmt.Errorf("ssg: asset file %s has unsupported extension %q", path, ext)
				}
			}

			rel, err := filepath.Rel(tree.dir, path)
			if err != nil {
				return err
			}
			for output, previous := range outputs {
				separator := string(filepath.Separator)
				if rel == output || strings.HasPrefix(rel, output+separator) || strings.HasPrefix(output, rel+separator) {
					return fmt.Errorf("ssg: output path collision between %s file %s and %s file %s", previous.kind, filepath.Join(previous.dir, output), tree.kind, path)
				}
			}
			outputs[rel] = tree
			return nil
		})
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func copyStaticFiles(outDir, inDir string) error {
	return generateFiles(outDir, inDir, false, func(out io.Writer, in io.Reader, _ string) error {
		_, err := io.Copy(out, in)
		return err
	})
}

func generateAssets(outDir, inDir string) error {
	return generateFiles(outDir, inDir, true, func(out io.Writer, in io.Reader, path string) error {
		switch filepath.Ext(path) {
		case ".css":
			return htmlrewrite.MinifyCSS(out, in)
		case ".js":
			return htmlrewrite.MinifyJS(out, in)
		default:
			panic("unreachable")
		}
	})
}

func generateFiles(outDir, inDir string, applyIgnoreRules bool, generate func(io.Writer, io.Reader, string) error) error {
	var wg errgroup.Group
	if err := filepath.Walk(inDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if applyIgnoreRules && isIgnoredFile(path) {
			return nil
		}
		wg.Go(func() error {
			inRelPath, err := filepath.Rel(inDir, path)
			if err != nil {
				return err
			}
			outPath := filepath.Join(outDir, inRelPath)
			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				return err
			}

			out, err := os.Create(outPath)
			if err != nil {
				return err
			}
			defer out.Close()

			in, err := os.Open(path)
			if err != nil {
				return err
			}
			defer in.Close()

			outbuf := bufio.NewWriter(out)
			if err := generate(outbuf, bufio.NewReader(in), path); err != nil {
				return err
			}
			if err := outbuf.Flush(); err != nil {
				return err
			}
			return nil
		})
		return nil
	}); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := wg.Wait(); err != nil {
		return err
	}
	return nil
}
