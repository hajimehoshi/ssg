// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package ssg

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func contentTitle(content []byte) (string, error) {
	nodes, err := html.ParseFragment(bytes.NewReader(content), &html.Node{
		Type:     html.ElementNode,
		Data:     "body",
		DataAtom: atom.Body,
	})
	if err != nil {
		return "", err
	}
	for _, node := range nodes {
		if h1 := firstElementByName(node, "h1"); h1 != nil {
			var title strings.Builder
			appendText(&title, h1)
			return strings.TrimSpace(title.String()), nil
		}
	}
	return "", nil
}

func firstElementByName(node *html.Node, name string) *html.Node {
	if node.Type == html.ElementNode && node.Data == name {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if element := firstElementByName(child, name); element != nil {
			return element
		}
	}
	return nil
}

func appendText(text *strings.Builder, node *html.Node) {
	if node.Type == html.TextNode {
		text.WriteString(node.Data)
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		appendText(text, child)
	}
}
