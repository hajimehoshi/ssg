// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package pagepath_test

import (
	"testing"

	"github.com/hajimehoshi/ssg/internal/pagepath"
)

func TestOutput(t *testing.T) {
	testCases := []struct {
		In  string
		Out string
	}{
		{In: "index.html", Out: "index.html"},
		{In: "index.md", Out: "index.html"},
		{In: "writings/article.md", Out: "writings/article.html"},
		{In: "article.markdown", Out: "article.markdown"},
	}
	for _, tc := range testCases {
		if got, want := pagepath.Output(tc.In), tc.Out; got != want {
			t.Errorf("Output(%q): got: %q, want: %q", tc.In, got, want)
		}
	}
}
