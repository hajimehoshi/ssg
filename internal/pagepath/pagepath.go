// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

// Package pagepath maps page source paths to generated paths.
package pagepath

import "strings"

// Output returns the generated path for a page source path.
func Output(path string) string {
	if before, ok := strings.CutSuffix(path, ".md"); ok {
		return before + ".html"
	}
	return path
}
