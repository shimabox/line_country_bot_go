// Command fetchflags downloads a flag PNG from Flagpedia (flagcdn.com) for
// every country in the embedded country data into public/flags, where Workers
// Static Assets serves them. It then decodes every flag and writes its pixel
// size and the colour of its top corners to internal/country/flags_gen.go. The
// bot uses the size to decide how to fit a flag into the image frame and the
// corner colour as the background of flags that fill the frame. The corner
// colour is the mean of a 3x3 square 2 pixels inside each top corner of the
// part shown in the frame (cropped centred), and the mean of the left and
// right colours when they differ. Run it from the repository root:
//
//	go run ./tools/fetchflags          # fetch missing files only
//	go run ./tools/fetchflags -force   # fetch everything again
//
// It exits with a non-zero status and lists the codes when a download fails
// or a flag cannot be decoded or is too small to sample.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shimabox/line_country_bot_go/internal/country"
)

const (
	defaultBase = "https://flagcdn.com"
	userAgent   = "line_country_bot_go-fetchflags/1.0 (https://github.com/shimabox/line_country_bot_go)"
	maxFlagSize = 1 << 20
)

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

func main() {
	dir := flag.String("dir", "public/flags", "output directory")
	base := flag.String("base", defaultBase, "flag CDN base URL")
	force := flag.Bool("force", false, "download files that already exist")
	gen := flag.String("gen", "internal/country/flags_gen.go", "Go file to write the flag sizes and corner colours to (empty to skip)")
	flag.Parse()

	if err := run(*dir, *base, *force, *gen); err != nil {
		fmt.Fprintln(os.Stderr, "fetchflags:", err)
		os.Exit(1)
	}
}

func run(dir, base string, force bool, genPath string) error {
	store := country.Default()
	codes := make([]string, 0, store.Len())
	for _, c := range store.All() {
		codes = append(codes, c.Code)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	res, err := Fetch(context.Background(), client, base, dir, codes, force)
	fmt.Fprintf(os.Stderr, "fetchflags: fetched %d, skipped %d, failed %d\n",
		len(res.Fetched), len(res.Skipped), len(res.Failed))
	if err != nil {
		return err
	}

	rep, err := Check(dir, codes)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "fetchflags: %d files, %d bytes in %s\n", len(codes)-len(rep.Missing), rep.TotalBytes, dir)
	if len(rep.Extra) > 0 {
		fmt.Fprintf(os.Stderr, "fetchflags: warning: files without a country (delete them): %s\n", strings.Join(rep.Extra, ", "))
	}
	if len(rep.Missing) > 0 {
		return fmt.Errorf("missing flags: %s", strings.Join(rep.Missing, ", "))
	}

	if genPath == "" {
		return nil
	}
	infos, err := Infos(dir, codes)
	if err != nil {
		return err
	}
	if err := writeInfosFile(genPath, infos); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "fetchflags: wrote %d flags to %s\n", len(infos), genPath)
	return nil
}

// writeInfosFile writes the flag info source to path through a temporary
// file. The directory must already exist, so running outside the repository
// root fails instead of creating a stray tree.
func writeInfosFile(path string, infos map[string]Info) error {
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err != nil {
		return fmt.Errorf("gen output directory: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("gen output directory %s is not a directory", dir)
	}
	var buf bytes.Buffer
	if err := WriteInfos(&buf, infos); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// FlagURL returns the 320px-wide PNG URL for an ISO code.
func FlagURL(base, code string) string {
	return strings.TrimRight(base, "/") + "/w320/" + strings.ToLower(code) + ".png"
}

// FlagPath returns where the flag for code is stored.
func FlagPath(dir, code string) string {
	return filepath.Join(dir, strings.ToLower(code)+".png")
}

// Result lists what Fetch did, in input order. Failed maps a code to the
// reason.
type Result struct {
	Fetched []string
	Skipped []string
	Failed  map[string]string
}

// Fetch downloads the flags one by one. Existing files are skipped unless
// force is set. Files are written only after a complete PNG is received.
func Fetch(ctx context.Context, client *http.Client, base, dir string, codes []string, force bool) (Result, error) {
	res := Result{Failed: map[string]string{}}
	for _, code := range codes {
		path := FlagPath(dir, code)
		if !force {
			if _, err := os.Stat(path); err == nil {
				res.Skipped = append(res.Skipped, code)
				continue
			}
		}
		if err := download(ctx, client, FlagURL(base, code), path); err != nil {
			res.Failed[code] = err.Error()
			continue
		}
		res.Fetched = append(res.Fetched, code)
	}
	if len(res.Failed) > 0 {
		var lines []string
		for _, code := range sortedKeys(res.Failed) {
			lines = append(lines, code+": "+res.Failed[code])
		}
		return res, errors.New("failed to fetch:\n  " + strings.Join(lines, "\n  "))
	}
	return res, nil
}

func download(ctx context.Context, client *http.Client, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFlagSize+1))
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	if len(data) > maxFlagSize {
		return fmt.Errorf("GET %s: larger than %d bytes", url, maxFlagSize)
	}
	if !bytes.HasPrefix(data, pngSignature) {
		return fmt.Errorf("GET %s: response is not a PNG", url)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Report describes the contents of the flag directory.
type Report struct {
	Missing    []string // codes without a file
	Extra      []string // files that belong to no code
	TotalBytes int64    // size of the files that belong to a code
}

// Check compares the directory with the expected codes.
func Check(dir string, codes []string) (Report, error) {
	var rep Report
	entries, err := os.ReadDir(dir)
	if err != nil {
		return rep, err
	}
	want := map[string]bool{}
	for _, code := range codes {
		want[strings.ToLower(code)+".png"] = true
	}
	present := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if !want[name] || e.IsDir() {
			rep.Extra = append(rep.Extra, name)
			continue
		}
		info, err := e.Info()
		if err != nil {
			return rep, err
		}
		present[name] = true
		rep.TotalBytes += info.Size()
	}
	for _, code := range codes {
		if !present[strings.ToLower(code)+".png"] {
			rep.Missing = append(rep.Missing, code)
		}
	}
	slices.Sort(rep.Extra)
	return rep, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
