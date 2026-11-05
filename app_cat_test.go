package main

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestCatEnvMovesDevServer(t *testing.T) {
	got := catEnv([]string{"PATH=x", "devserver=localhost:34115", "frontenddevserverurl=http://localhost:5173"})
	want := []string{"PATH=x", "devserver=127.0.0.1:34116", "frontenddevserverurl=http://localhost:5173"}
	if !slices.Equal(got, want) {
		t.Fatalf("catEnv = %v, want %v", got, want)
	}
}

func TestParseCatArgs(t *testing.T) {
	got := parseCatArgs([]string{"--cat", "--app=1000x640", "--max", "--light"})
	want := CatSetup{AppWidth: 1000, AppHeight: 640, Maximised: true, Light: true}
	if got != want {
		t.Fatalf("parseCatArgs = %+v, want %+v", got, want)
	}
	if got := parseCatArgs([]string{"--cat", "--app=oops"}); got.AppWidth != 1180 || got.AppHeight != 740 {
		t.Fatalf("плохой --app должен давать размер по умолчанию, а дал %+v", got)
	}
}

func TestCatPageServesCatHTML(t *testing.T) {
	for path, want := range map[string]string{"/": "/cat.html", "/index.html": "/cat.html", "/assets/a.js": "/assets/a.js"} {
		var seen string
		h := catPage(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.URL.Path }))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
		if seen != want {
			t.Errorf("%s → %s, want %s", path, seen, want)
		}
	}
}
