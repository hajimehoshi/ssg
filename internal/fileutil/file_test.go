// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package fileutil_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/hajimehoshi/ssg/internal/fileutil"
)

func TestHashUsesLowerCaseBase32(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	hash, err := fileutil.Hash(path)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[a-z2-7]{10}$`).MatchString(hash) {
		t.Errorf("Hash(%q) = %q, want a 10-character lower-case base32 hash", path, hash)
	}
}
