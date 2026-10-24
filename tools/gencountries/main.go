// Command gencountries regenerates internal/country/data/countries.json from
// Wikidata. Run it from the repository root:
//
//	go run ./tools/gencountries
//
// Problems that need a manual decision (several capitals, a display name that
// is too long, ...) are listed with the country code and the reason; resolve
// them in tools/gencountries/overrides.json and run again.
package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

//go:embed query.sparql
var query string

const (
	endpoint  = "https://query.wikidata.org/sparql"
	userAgent = "line_country_bot_go-gencountries/1.0 (https://github.com/shimabox/line_country_bot_go)"
)

func main() {
	out := flag.String("out", "internal/country/data/countries.json", "output file")
	overridesPath := flag.String("overrides", "tools/gencountries/overrides.json", "overrides file")
	input := flag.String("input", "", "read SPARQL JSON results from this file instead of querying Wikidata")
	date := flag.String("date", time.Now().UTC().Format("2006-01-02"), "value of generated_at")
	flag.Parse()

	if err := run(*out, *overridesPath, *input, *date); err != nil {
		fmt.Fprintln(os.Stderr, "gencountries:", err)
		os.Exit(1)
	}
}

func run(outPath, overridesPath, input, date string) error {
	ovData, err := os.ReadFile(overridesPath)
	if err != nil {
		return err
	}
	ov, err := ParseOverrides(ovData)
	if err != nil {
		return err
	}

	var raw []byte
	if input != "" {
		raw, err = os.ReadFile(input)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		raw, err = fetch(ctx, http.DefaultClient, query)
	}
	if err != nil {
		return err
	}
	res, err := ParseResults(raw)
	if err != nil {
		return err
	}

	f, err := Build(res, ov, date)
	if err != nil {
		return err
	}
	data, err := Encode(f)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gencountries: wrote %d countries from %d rows to %s\n",
		len(f.Countries), len(res.Results.Bindings), outPath)
	for _, s := range SharedAliases(f) {
		fmt.Fprintf(os.Stderr, "gencountries: warning: alias shared by several countries is not searchable: %s\n", s)
	}
	return nil
}

// fetch runs the query against the Wikidata Query Service. The User-Agent
// identifies the tool as the service's policy requires.
func fetch(ctx context.Context, client *http.Client, q string) ([]byte, error) {
	form := url.Values{"query": {q}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query Wikidata: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Wikidata response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if len(body) > 512 {
			body = body[:512]
		}
		return nil, errors.New("query Wikidata: status " + resp.Status + ": " + string(body))
	}
	return body, nil
}
