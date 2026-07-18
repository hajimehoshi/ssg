// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023 Hajime Hoshi

package htmlrewrite

import (
	"net/url"

	"golang.org/x/net/html"

	"github.com/hajimehoshi/ssg/internal/fileutil"
)

// AddResourceVersions adds a content hash to every local resource filename and
// returns versioned destinations mapped to their source files.
func AddResourceVersions(node *html.Node, outDir, pageDir string) (map[string]string, error) {
	versions := map[string]string{}
	if err := addResourceVersions(node, outDir, pageDir, versions); err != nil {
		return nil, err
	}
	return versions, nil
}

func addResourceVersions(node *html.Node, outDir, pageDir string, versions map[string]string) error {
	if node.Type == html.ElementNode {
		for i := range node.Attr {
			if !isResourceAttr(node, node.Attr[i].Key) {
				continue
			}
			v, source, destination, err := versionedURL(node.Attr[i].Val, outDir, pageDir)
			if err != nil {
				return err
			}
			node.Attr[i].Val = v
			if source != "" {
				versions[destination] = source
			}
		}
	}
	for n := node.FirstChild; n != nil; n = n.NextSibling {
		if err := addResourceVersions(n, outDir, pageDir, versions); err != nil {
			return err
		}
	}
	return nil
}

// versionedURL returns rawURL with a content hash in its filename. URLs that do
// not point at a local file under outDir are returned unchanged.
func versionedURL(rawURL, outDir, pageDir string) (string, string, string, error) {
	file, ok := localFilePath(rawURL, outDir, pageDir)
	if !ok {
		return rawURL, "", "", nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, "", "", nil
	}
	h, err := fileutil.Hash(file)
	if err != nil {
		return "", "", "", err
	}

	u.Path = fileutil.VersionedPath(u.Path, h)
	u.RawPath = ""
	return u.String(), file, fileutil.VersionedPath(file, h), nil
}
