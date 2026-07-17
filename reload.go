// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package ssg

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/template"
)

const notifyPath = "/_ssg/notify"

const reloadScriptSource = `
(async () => {
  const response = await fetch("{{ .NotifyPath }}?gen={{ .Gen }}", {
    cache: "no-store",
  });
  if (response.ok) {
    location.reload();
  }
})();
`

var reloadScriptTemplate = func() *template.Template {
	var result bytes.Buffer
	if err := minifyJS(&result, strings.NewReader(reloadScriptSource)); err != nil {
		panic(err)
	}
	return template.Must(template.New("reload").Parse(result.String()))
}()

type serveState struct {
	mu   sync.RWMutex
	cond *sync.Cond
	gen  uint64
}

func newServeState() *serveState {
	state := &serveState{}
	state.cond = sync.NewCond(&state.mu)
	return state
}

func (s *serveState) withSite(f func(gen uint64)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f(s.gen)
}

func (s *serveState) regenerate(options *GenerateOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := Generate(options); err != nil {
		return err
	}
	s.gen++
	s.cond.Broadcast()
	return nil
}

func (s *serveState) broadcastCancellation() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Holding mu prevents cancellation from being lost between the wait
	// predicate check and Cond.Wait.
	s.cond.Broadcast()
}

func (s *serveState) wait(ctx context.Context, gen uint64) bool {
	stop := context.AfterFunc(ctx, s.broadcastCancellation)
	defer stop()

	s.mu.Lock()
	defer s.mu.Unlock()
	for gen == s.gen && ctx.Err() == nil {
		s.cond.Wait()
	}
	return ctx.Err() == nil
}

func (h handler) notify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	gen, err := strconv.ParseUint(r.URL.Query().Get("gen"), 10, 64)
	if err != nil {
		http.Error(w, "invalid gen", http.StatusBadRequest)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	if h.state.wait(r.Context(), gen) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
}

func (h handler) serveHTML(w http.ResponseWriter, r *http.Request, path string, info os.FileInfo, gen uint64) {
	content, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, path, info.ModTime(), bytes.NewReader(injectReloadScript(content, gen)))
}

func injectReloadScript(content []byte, gen uint64) []byte {
	index := bytes.LastIndex(bytes.ToLower(content), []byte("</body>"))
	if index < 0 {
		index = len(content)
	}

	var result bytes.Buffer
	result.Grow(len(content) + len("<script></script>"))
	result.Write(content[:index])
	result.WriteString("<script>")
	if err := reloadScriptTemplate.Execute(&result, struct {
		NotifyPath string
		Gen        uint64
	}{
		NotifyPath: notifyPath,
		Gen:        gen,
	}); err != nil {
		panic(err)
	}
	result.WriteString("</script>")
	result.Write(content[index:])
	return result.Bytes()
}
