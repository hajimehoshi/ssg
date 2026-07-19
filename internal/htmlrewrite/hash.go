// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package htmlrewrite

import (
	"bufio"
	"encoding/base32"
	"hash/fnv"
	"io"
	"os"
	"strings"
)

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := fnv.New128a()
	if _, err := io.Copy(h, bufio.NewReader(f)); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.EncodeToString(h.Sum(nil))[:10]), nil
}
