// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package fileutil

import (
	"bufio"
	"encoding/base64"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Copy copies src to dst.
func Copy(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// Hash returns a filename-safe content hash of path.
func Hash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := fnv.New128a()
	if _, err := io.Copy(h, bufio.NewReader(f)); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(h.Sum(nil))[:10], nil
}

// VersionedPath inserts hash before the extension in path.
func VersionedPath(path, hash string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + "." + hash + ext
}
