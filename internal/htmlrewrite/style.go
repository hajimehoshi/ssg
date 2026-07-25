// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package htmlrewrite

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"golang.org/x/net/html"
)

// MinifyCSS writes the minified form of the CSS read from in to out.
func MinifyCSS(out io.Writer, in io.Reader) error {
	css, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	r := api.Transform(string(css), api.TransformOptions{
		Loader:            api.LoaderCSS,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
	})
	if len(r.Errors) > 0 {
		var msgs []string
		for _, e := range r.Errors {
			msgs = append(msgs, e.Text)
		}
		return fmt.Errorf("ssg: minifying CSS failed: %s", strings.Join(msgs, ", "))
	}
	if _, err := out.Write(bytes.TrimSpace(r.Code)); err != nil {
		return err
	}
	return nil
}

func minifyStyleElements(node *html.Node) error {
	if node.Type == html.ElementNode && node.Data == "style" {
		var css strings.Builder
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.TextNode {
				css.WriteString(child.Data)
			}
		}

		var minified strings.Builder
		if err := MinifyCSS(&minified, strings.NewReader(css.String())); err != nil {
			return err
		}

		for node.FirstChild != nil {
			node.RemoveChild(node.FirstChild)
		}
		if minified.Len() > 0 {
			node.AppendChild(&html.Node{
				Type: html.TextNode,
				Data: minified.String(),
			})
		}
		return nil
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := minifyStyleElements(child); err != nil {
			return err
		}
	}
	return nil
}
