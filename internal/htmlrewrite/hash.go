// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package htmlrewrite

import (
	"bufio"
	"crypto/sha256"
	"encoding/base32"
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

	h := sha256.New()
	if _, err := io.Copy(h, bufio.NewReader(f)); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.EncodeToString(h.Sum(nil))[:10]), nil
}
