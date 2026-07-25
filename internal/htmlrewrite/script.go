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

// MinifyJS writes the minified form of the JavaScript read from in to out.
func MinifyJS(out io.Writer, in io.Reader) error {
	js, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	r := api.Transform(string(js), api.TransformOptions{
		Loader:            api.LoaderJS,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
	})
	if len(r.Errors) > 0 {
		var msgs []string
		for _, e := range r.Errors {
			msgs = append(msgs, e.Text)
		}
		return fmt.Errorf("ssg: minifying JS failed: %s", strings.Join(msgs, ", "))
	}
	if _, err := out.Write(bytes.TrimSpace(r.Code)); err != nil {
		return err
	}
	return nil
}

func minifyScriptElements(node *html.Node) error {
	if node.Type == html.ElementNode && node.Data == "script" && hasJavaScriptType(node) {
		var js strings.Builder
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.TextNode {
				js.WriteString(child.Data)
			}
		}

		var minified strings.Builder
		if err := MinifyJS(&minified, strings.NewReader(js.String())); err != nil {
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
		if err := minifyScriptElements(child); err != nil {
			return err
		}
	}
	return nil
}

func hasJavaScriptType(node *html.Node) bool {
	for _, attr := range node.Attr {
		if attr.Key != "type" {
			continue
		}
		if attr.Val == "" || strings.EqualFold(attr.Val, "module") {
			return true
		}
		for _, mimeType := range []string{
			"application/ecmascript",
			"application/javascript",
			"application/x-ecmascript",
			"application/x-javascript",
			"text/ecmascript",
			"text/javascript",
			"text/javascript1.0",
			"text/javascript1.1",
			"text/javascript1.2",
			"text/javascript1.3",
			"text/javascript1.4",
			"text/javascript1.5",
			"text/jscript",
			"text/livescript",
			"text/x-ecmascript",
			"text/x-javascript",
		} {
			if strings.EqualFold(attr.Val, mimeType) {
				return true
			}
		}
		return false
	}
	return true
}
