package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/shimabox/line_country_bot_go/internal/country"
)

var fakePNG = append([]byte("\x89PNG\r\n\x1a\n"), []byte("fake image data")...)

type flagServer struct {
	mu    sync.Mutex
	paths []string
	srv   *httptest.Server
}

// newFlagServer serves a PNG for every code except those in missing, and
// non-PNG content for those in broken.
func newFlagServer(t *testing.T, missing, broken []string) *flagServer {
	fs := &flagServer{}
	fs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		fs.paths = append(fs.paths, r.URL.Path)
		fs.mu.Unlock()
		if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "line_country_bot_go") {
			http.Error(w, "no user agent", http.StatusForbidden)
			return
		}
		for _, m := range missing {
			if r.URL.Path == "/w320/"+m+".png" {
				http.NotFound(w, r)
				return
			}
		}
		for _, b := range broken {
			if r.URL.Path == "/w320/"+b+".png" {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte("<html>not a png</html>"))
				return
			}
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(fakePNG)
	}))
	t.Cleanup(fs.srv.Close)
	return fs
}

func (fs *flagServer) requested() []string {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return append([]string(nil), fs.paths...)
}

func TestFlagURLAndPath(t *testing.T) {
	if got := FlagURL("https://flagcdn.com", "JP"); got != "https://flagcdn.com/w320/jp.png" {
		t.Errorf("FlagURL = %s", got)
	}
	if got := FlagURL("https://flagcdn.com/", "GB"); got != "https://flagcdn.com/w320/gb.png" {
		t.Errorf("FlagURL trailing slash = %s", got)
	}
	if got := FlagPath("public/flags", "XK"); got != filepath.Join("public", "flags", "xk.png") {
		t.Errorf("FlagPath = %s", got)
	}
}

func TestFetchDownloadsAndSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(FlagPath(dir, "FR"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := newFlagServer(t, nil, nil)
	res, err := Fetch(context.Background(), fs.srv.Client(), fs.srv.URL, dir, []string{"JP", "FR", "US"}, false)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !reflect.DeepEqual(res.Fetched, []string{"JP", "US"}) || !reflect.DeepEqual(res.Skipped, []string{"FR"}) || len(res.Failed) != 0 {
		t.Errorf("result = %+v", res)
	}
	got, _ := os.ReadFile(FlagPath(dir, "JP"))
	if !bytes.Equal(got, fakePNG) {
		t.Errorf("jp.png = %q", got)
	}
	if got, _ := os.ReadFile(FlagPath(dir, "FR")); string(got) != "existing" {
		t.Errorf("existing file overwritten: %q", got)
	}
	paths := fs.requested()
	if !reflect.DeepEqual(paths, []string{"/w320/jp.png", "/w320/us.png"}) {
		t.Errorf("requested = %v", paths)
	}
}

func TestFetchForce(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(FlagPath(dir, "FR"), []byte("existing"), 0o644)
	fs := newFlagServer(t, nil, nil)
	res, err := Fetch(context.Background(), fs.srv.Client(), fs.srv.URL, dir, []string{"FR"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Fetched, []string{"FR"}) {
		t.Errorf("result = %+v", res)
	}
	if got, _ := os.ReadFile(FlagPath(dir, "FR")); !bytes.Equal(got, fakePNG) {
		t.Errorf("fr.png not replaced: %q", got)
	}
}

func TestFetchReportsFailures(t *testing.T) {
	dir := t.TempDir()
	fs := newFlagServer(t, []string{"xk"}, []string{"tw"})
	res, err := Fetch(context.Background(), fs.srv.Client(), fs.srv.URL, dir, []string{"XK", "JP", "TW"}, false)
	if err == nil {
		t.Fatal("err = nil, want failure")
	}
	if !strings.Contains(err.Error(), "TW") || !strings.Contains(err.Error(), "XK") {
		t.Errorf("error does not list the codes: %v", err)
	}
	if !reflect.DeepEqual(res.Fetched, []string{"JP"}) || !reflect.DeepEqual(sortedKeys(res.Failed), []string{"TW", "XK"}) {
		t.Errorf("result = %+v", res)
	}
	for _, code := range []string{"XK", "TW"} {
		if _, err := os.Stat(FlagPath(dir, code)); !os.IsNotExist(err) {
			t.Errorf("%s: file written for a failed download", code)
		}
	}
}

func TestCheck(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"jp.png", "fr.png", "zz.png", "README.txt"} {
		os.WriteFile(filepath.Join(dir, name), fakePNG, 0o644)
	}
	rep, err := Check(dir, []string{"JP", "FR", "US"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rep.Missing, []string{"US"}) {
		t.Errorf("Missing = %v", rep.Missing)
	}
	if !reflect.DeepEqual(rep.Extra, []string{"README.txt", "zz.png"}) {
		t.Errorf("Extra = %v", rep.Extra)
	}
	if rep.TotalBytes != int64(2*len(fakePNG)) {
		t.Errorf("TotalBytes = %d", rep.TotalBytes)
	}
}

// TestPublicFlagsComplete fails when public/flags misses a flag for a code in
// the embedded country data, holds files for unknown codes, or grows past the
// size budget. Run fetchflags to fix missing files.
func TestPublicFlagsComplete(t *testing.T) {
	store, err := country.Default()
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, c := range store.All() {
		codes = append(codes, c.Code)
	}
	rep, err := Check(filepath.Join("..", "..", "public", "flags"), codes)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Missing) > 0 {
		t.Errorf("missing flags (run go run ./tools/fetchflags): %v", rep.Missing)
	}
	if len(rep.Extra) > 0 {
		t.Errorf("files without a country: %v", rep.Extra)
	}
	if rep.TotalBytes >= 1_000_000 {
		t.Errorf("flags total %d bytes, want < 1MB", rep.TotalBytes)
	}
}
