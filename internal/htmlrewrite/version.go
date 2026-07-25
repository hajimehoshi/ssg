// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023 Hajime Hoshi

package htmlrewrite

import (
	"errors"
	"net/url"
	"os"

	"golang.org/x/net/html"
)

// MissingResourceMode controls how missing local resources are handled.
type MissingResourceMode int

const (
	ErrorOnMissingResource MissingResourceMode = iota
	IgnoreMissingResource
)

// AddResourceVersions appends a content hash query to every local resource URL.
// mode controls missing resources.
func AddResourceVersions(node *html.Node, outDir, pageDir string, mode MissingResourceMode) error {
	if node.Type == html.ElementNode {
		for i := range node.Attr {
			if !isResourceAttr(node, node.Attr[i].Key) {
				continue
			}
			v, err := versionedURL(node.Attr[i].Val, outDir, pageDir, mode)
			if err != nil {
				return err
			}
			node.Attr[i].Val = v
		}
	}
	for n := node.FirstChild; n != nil; n = n.NextSibling {
		if err := AddResourceVersions(n, outDir, pageDir, mode); err != nil {
			return err
		}
	}
	return nil
}

// versionedURL returns rawURL with a content hash query. URLs that do
// not point at a local file under outDir are returned unchanged.
func versionedURL(rawURL, outDir, pageDir string, mode MissingResourceMode) (string, error) {
	file, ok := localFilePath(rawURL, outDir, pageDir)
	if !ok {
		return rawURL, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, nil
	}
	h, err := fileHash(file)
	if mode == IgnoreMissingResource && errors.Is(err, os.ErrNotExist) {
		return rawURL, nil
	}
	if err != nil {
		return "", err
	}

	q := u.Query()
	q.Set("v", h)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
