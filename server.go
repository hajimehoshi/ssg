// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023 Hajime Hoshi

package ssg

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	// changes to the contents directory.
	GenerateOptions GenerateOptions
}

// ServeSite generates and serves the site, regenerating it when contents
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

	server := &http.Server{
		Addr: options.Addr,
		Handler: handler{
			rootPath:          outDir,
			keepHTMLExtension: options.GenerateOptions.KeepHTMLExtension,
		},
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return watch(groupCtx, &options.GenerateOptions)
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
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The generator omits the .html extension from page URLs by default, so the
	// .html URL must not serve the page at a second URL.
	// http.ServeFile already redirects .../index.html.
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
			h.notFound(w, r)
			return
		}
		// The generator omits the .html extension from page URLs by
		// default, so an extensionless URL must reach its .html file.
		path += ".html"
		f, err = os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				h.notFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// The generator advertises a directory's index page as the directory
		// itself, so .../index must not serve the page at a second URL.
		// http.ServeFile already redirects .../index.html.
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
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				h.notFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeFile(w, r, path)
}

func (h handler) notFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)

	f, err := os.Open(filepath.Join(h.rootPath, "404.html"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	io.Copy(w, f)
}
