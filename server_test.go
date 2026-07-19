// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package ssg_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ssg"
)

func newTestSite(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	for _, path := range []string{
		"index.html",
		"404.html",
		"writings/index.html",
		"writings/foo.html",
		"style.css",
	} {
		path = filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(path), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestHandler(t *testing.T) {
	testCases := []struct {
		Name              string
		In                string
		KeepHTMLExtension bool
		Code              int
		Location          string
	}{
		{
			Name: "root",
			In:   "/",
			Code: http.StatusOK,
		},
		{
			Name: "page",
			In:   "/writings/foo",
			Code: http.StatusOK,
		},
		{
			Name:              "page, keeping the extension",
			In:                "/writings/foo",
			KeepHTMLExtension: true,
			Code:              http.StatusNotFound,
		},
		{
			Name:     "page with the extension",
			In:       "/writings/foo.html",
			Code:     http.StatusMovedPermanently,
			Location: "/writings/foo",
		},
		{
			Name:     "page with the extension and a query",
			In:       "/writings/foo.html?a=b",
			Code:     http.StatusMovedPermanently,
			Location: "/writings/foo?a=b",
		},
		{
			Name:              "page with the extension, keeping the extension",
			In:                "/writings/foo.html",
			KeepHTMLExtension: true,
			Code:              http.StatusOK,
		},
		{
			Name:     "index",
			In:       "/writings/index",
			Code:     http.StatusMovedPermanently,
			Location: "/writings/",
		},
		{
			Name:              "index, keeping the extension",
			In:                "/writings/index",
			KeepHTMLExtension: true,
			Code:              http.StatusNotFound,
		},
		{
			Name:     "index with the extension",
			In:       "/writings/index.html",
			Code:     http.StatusMovedPermanently,
			Location: "./",
		},
		{
			Name:              "index with the extension, keeping the extension",
			In:                "/writings/index.html",
			KeepHTMLExtension: true,
			Code:              http.StatusMovedPermanently,
			Location:          "./",
		},
		{
			Name:     "root index with the extension",
			In:       "/index.html",
			Code:     http.StatusMovedPermanently,
			Location: "./",
		},
		{
			Name: "non-page file",
			In:   "/style.css",
			Code: http.StatusOK,
		},
		{
			Name: "missing page",
			In:   "/writings/bar",
			Code: http.StatusNotFound,
		},
		{
			Name:     "missing page with the extension",
			In:       "/writings/bar.html",
			Code:     http.StatusMovedPermanently,
			Location: "/writings/bar",
		},
	}

	dir := newTestSite(t)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			h := ssg.NewHandler(dir, tc.KeepHTMLExtension)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.In, nil))

			if got, want := w.Code, tc.Code; got != want {
				t.Errorf("code: got: %d, want: %d", got, want)
			}
			if got, want := w.Header().Get("Location"), tc.Location; got != want {
				t.Errorf("location: got: %q, want: %q", got, want)
			}
		})
	}
}

func TestServeSite(t *testing.T) {
	dir := t.TempDir()
	writeProjectSite(t, dir)

	addr := unusedLocalAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- ssg.ServeSite(ctx, &ssg.ServeSiteOptions{
			Addr: addr,
			GenerateOptions: ssg.GenerateOptions{
				Dir:      dir,
				SiteName: "Test",
			},
		})
	}()

	url := "http://" + addr + "/"
	waitForHTTPContent(t, url, "one")
	initial := getHTTPContent(t, url)
	initialGen := reloadGeneration(t, initial)

	notifyCh := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/_ssg/notify?gen="+initialGen, nil)
		if err != nil {
			notifyCh <- err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			notifyCh <- err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			notifyCh <- fmt.Errorf("notification status: got: %d, want: %d", resp.StatusCode, http.StatusNoContent)
			return
		}
		notifyCh <- nil
	}()
	waitForServedRegeneration(t, filepath.Join(dir, "src", "content", "index.html"), url)
	select {
	case err := <-notifyCh:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("notification did not return after regeneration")
	}

	regenerated := getHTTPContent(t, url)
	if got := reloadGeneration(t, regenerated); got == initialGen {
		t.Errorf("generation did not advance from %s", initialGen)
	}

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("ServeSite: got: %v, want: %v", err, context.Canceled)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ServeSite did not return after its context was canceled")
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("ServeSite did not release its address: %v", err)
	}
	l.Close()
}

func TestNotifyStopsWhenRequestContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	h := ssg.NewHandler(newTestSite(t), false)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		r := httptest.NewRequest(http.MethodGet, "/_ssg/notify?gen=0", nil).WithContext(ctx)
		h.ServeHTTP(w, r)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("notification did not stop after the request context was canceled")
	}
	if got, want := w.Code, http.StatusServiceUnavailable; got != want {
		t.Errorf("status: got: %d, want: %d", got, want)
	}
}

func TestServeSiteStopsWhenRegenerationFails(t *testing.T) {
	t.Chdir(t.TempDir())
	writeProjectSite(t, ".")

	addr := unusedLocalAddr(t)
	ctx := t.Context()
	errCh := make(chan error, 1)
	go func() {
		errCh <- ssg.ServeSite(ctx, &ssg.ServeSiteOptions{
			Addr: addr,
			GenerateOptions: ssg.GenerateOptions{
				SiteName: "Test",
			},
		})
	}()
	waitForHTTPContent(t, "http://"+addr+"/", "one")

	if err := os.WriteFile(filepath.Join("src", "layouts", "default.html"), []byte("{{"), 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("ServeSite succeeded after regeneration failed")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ServeSite did not return after regeneration failed")
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("ServeSite did not stop the HTTP server: %v", err)
	}
	l.Close()
}

func TestServeSiteStopsWatchingWhenServerFails(t *testing.T) {
	t.Chdir(t.TempDir())
	writeProjectSite(t, ".")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	err = ssg.ServeSite(context.Background(), &ssg.ServeSiteOptions{
		Addr: l.Addr().String(),
		GenerateOptions: ssg.GenerateOptions{
			SiteName: "Test",
		},
	})
	if err == nil {
		t.Fatal("ServeSite succeeded when its address was already in use")
	}
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		t.Errorf("ServeSite: got error type %T, want *net.OpError", err)
	}
}

func writeProjectSite(t *testing.T, dir string) {
	t.Helper()

	contentDir := filepath.Join(dir, "src", "content")
	layoutDir := filepath.Join(dir, "src", "layouts")
	for _, dir := range []string{contentDir, layoutDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(layoutDir, "default.html"), []byte("<html><body>{{.Page.Content}}</body></html>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "index.html"), []byte("<p>one</p>"), 0644); err != nil {
		t.Fatal(err)
	}
}

func unusedLocalAddr(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func waitForHTTPContent(t *testing.T, url, content string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr == nil && resp.StatusCode == http.StatusOK && strings.Contains(string(body), content) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s did not serve content containing %q", url, content)
}

func getHTTPContent(t *testing.T, url string) string {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: got status: %d, want: %d", url, resp.StatusCode, http.StatusOK)
	}
	return string(content)
}

func reloadGeneration(t *testing.T, content string) string {
	t.Helper()

	const marker = `/_ssg/notify?gen=`
	index := strings.Index(content, marker)
	if index < 0 {
		t.Fatalf("served page does not contain the reload script: %q", content)
	}
	start := index + len(marker)
	end := start
	for end < len(content) && content[end] >= '0' && content[end] <= '9' {
		end++
	}
	if end == start {
		t.Fatalf("reload script does not contain a generation: %q", content)
	}
	return content[start:end]
}

func waitForServedRegeneration(t *testing.T, contentPath, url string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := os.WriteFile(contentPath, []byte("<p>two</p>"), 0644); err != nil {
			t.Fatal(err)
		}
		resp, err := http.Get(url)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr == nil && resp.StatusCode == http.StatusOK && strings.Contains(string(body), "two") {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s was not regenerated and served", contentPath)
}
