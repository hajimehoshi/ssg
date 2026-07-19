// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023 Hajime Hoshi

package ssg

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"
)

// ServeSiteOptions is options for ServeSite.
type ServeSiteOptions struct {
	// Addr is the TCP address to listen on, in the form accepted by net.Listen.
	Addr string

	// GenerateOptions specifies how to generate the site initially and after
	// changes to the content or layout directories.
	GenerateOptions GenerateOptions
}

// ServeSite generates and serves the site, regenerating it when source files
// change. It blocks until ctx is canceled or watching, generation, or serving
// fails.
func ServeSite(ctx context.Context, options *ServeSiteOptions) error {
	if options == nil || options.Addr == "" {
		return fmt.Errorf("ssg: Addr must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := Generate(&options.GenerateOptions); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	group, groupCtx := errgroup.WithContext(ctx)
	state := newServeState()
	server := &http.Server{
		Addr: options.Addr,
		BaseContext: func(net.Listener) context.Context {
			return groupCtx
		},
		Handler: handler{
			rootPath:          options.GenerateOptions.outputDir(),
			keepHTMLExtension: options.GenerateOptions.KeepHTMLExtension,
			state:             state,
		},
	}
	group.Go(func() error {
		return watch(groupCtx, &options.GenerateOptions, state)
	})
	group.Go(func() error {
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
	group.Go(func() error {
		<-groupCtx.Done()
		return server.Shutdown(context.Background())
	})
	return group.Wait()
}

type handler struct {
	rootPath          string
	keepHTMLExtension bool
	state             *serveState
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == notifyPath {
		h.notify(w, r)
		return
	}
	// Keep the generated files and their generation stable while the response
	// reads them.
	h.state.withSite(func(gen uint64) {
		h.serveSite(w, r, gen)
	})
}

func (h handler) serveSite(w http.ResponseWriter, r *http.Request, gen uint64) {
	if strings.HasSuffix(r.URL.Path, "/index.html") {
		localRedirect(w, r, "./")
		return
	}

	// The generator omits the .html extension from page URLs by default, so the
	// .html URL must not serve the page at a second URL.
	if !h.keepHTMLExtension && strings.HasSuffix(r.URL.Path, ".html") && !strings.HasSuffix(r.URL.Path, "/index.html") {
		u := *r.URL
		u.Path = strings.TrimSuffix(u.Path, ".html")
		http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
		return
	}

	path := filepath.Join(h.rootPath, r.URL.Path[1:])
	f, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if h.keepHTMLExtension {
			h.notFound(w, r, gen)
			return
		}
		// The generator omits the .html extension from page URLs by
		// default, so an extensionless URL must reach its .html file.
		path += ".html"
		f, err = os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				h.notFound(w, r, gen)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// The generator advertises a directory's index page as the directory
		// itself, so .../index must not serve the page at a second URL.
		if strings.HasSuffix(r.URL.Path, "/index") {
			dir := "./"
			if r.URL.RawQuery != "" {
				dir += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, dir, http.StatusMovedPermanently)
			return
		}
	}

	if f.IsDir() {
		path = filepath.Join(path, "index.html")
		f, err = os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				h.notFound(w, r, gen)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	if filepath.Ext(path) == ".html" {
		h.serveHTML(w, r, path, f, gen)
		return
	}
	http.ServeFile(w, r, path)
}

func localRedirect(w http.ResponseWriter, r *http.Request, path string) {
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	w.Header().Set("Location", path)
	w.WriteHeader(http.StatusMovedPermanently)
}

func (h handler) notFound(w http.ResponseWriter, r *http.Request, gen uint64) {
	content, err := os.ReadFile(filepath.Join(h.rootPath, "404.html"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	content = injectReloadScript(content, gen)
	w.Header().Set("Cache-Control", "no-store")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		w.Write(content)
	}
}
