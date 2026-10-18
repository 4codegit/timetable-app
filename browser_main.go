//go:build browser

package main

// Browser dev launcher. The Wails desktop build needs gtk3/webkit2gtk
// development headers (gtk3-devel, webkit2gtk4.1-devel on Fedora), which are
// not always installed; this variant runs the exact same App methods behind a
// small HTTP server and serves frontend/dist so the full application works in
// a regular browser:
//
//	go build -tags browser -o /tmp/timetable-browser .
//	/tmp/timetable-browser
//
// Build with plain `go build .` (no tags) or `wails build` for the desktop app;
// the two variants never compile together (see the build tags on main.go).

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"timetable/internal/db"
)

//go:embed browser_shim.js
var browserShim []byte

// newBrowserApp mirrors main.go's NewApp for the browser build.
func newBrowserApp() *App {
	store, err := db.New(dbPath())
	if err != nil {
		log.Fatalf("db init failed: %v", err)
	}
	return &App{store: store}
}

func main() {
	app := newBrowserApp()
	app.startup(context.Background()) // a.ctx: solver cancellation, no GUI

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/rpc/")
		method := reflect.ValueOf(app).MethodByName(name)
		if !method.IsValid() {
			http.Error(w, "no bound method "+name, http.StatusNotFound)
			return
		}
		var raw []json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mt := method.Type()
		if len(raw) != mt.NumIn() {
			http.Error(w, fmt.Sprintf("%s wants %d args, got %d", name, mt.NumIn(), len(raw)), http.StatusBadRequest)
			return
		}
		in := make([]reflect.Value, mt.NumIn())
		for i := range in {
			p := reflect.New(mt.In(i))
			if err := json.Unmarshal(raw[i], p.Interface()); err != nil {
				http.Error(w, fmt.Sprintf("arg %d: %v", i+1, err), http.StatusBadRequest)
				return
			}
			in[i] = p.Elem()
		}
		out := method.Call(in)
		w.Header().Set("Content-Type", "application/json")
		if n := mt.NumOut(); n > 0 {
			if e, ok := out[n-1].Interface().(error); ok && e != nil {
				http.Error(w, e.Error(), http.StatusInternalServerError)
				return
			}
		}
		if mt.NumOut() > 1 || (mt.NumOut() == 1 && mt.Out(0).String() != "error") {
			if err := json.NewEncoder(w).Encode(out[0].Interface()); err != nil {
				log.Printf("%s: encode reply: %v", name, err)
			}
			return
		}
		w.Write([]byte("null"))
	})

	mux.HandleFunc("/shim.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Write(browserShim)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Inject the shim before any module script runs: window.go must
		// exist by the time the Svelte bundle's imports execute. Served
		// from the repo root or the binary's parent (build/) alike.
		dist := "frontend/dist"
		if _, err := os.Stat(dist); err != nil {
			dist = "../frontend/dist"
		}
		if _, err := os.Stat(dist); err != nil {
			// Windows/переносной запуск: папка dist рядом с exe.
			if exe, e := os.Executable(); e == nil {
				cand := filepath.Join(filepath.Dir(exe), "frontend", "dist")
				if _, err2 := os.Stat(cand); err2 == nil {
					dist = cand
				}
			}
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			b, err := os.ReadFile(filepath.Join(dist, "index.html"))
			if err != nil {
				http.Error(w, dist+"/index.html: "+err.Error()+" (run npm run build)", http.StatusInternalServerError)
				return
			}
			html := strings.Replace(string(b), "<head>", "<head><script src=\"/shim.js\"></script>", 1)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(html))
			return
		}
		http.FileServer(http.Dir(dist)).ServeHTTP(w, r)
	})

	addr := "127.0.0.1:8777"
	log.Printf("Timetable (browser mode) → http://%s — same timetable.db as the desktop app", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
