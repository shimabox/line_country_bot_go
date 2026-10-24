//go:build js && wasm

// Command worker is the Cloudflare Workers entry point. It only wires the
// Workers runtime (environment and fetch) into the host-testable handler.
package main

import (
	"net/http"

	"github.com/syumai/workers-go"
	"github.com/syumai/workers-go/cloudflare"
	"github.com/syumai/workers-go/cloudflare/fetch"

	"github.com/shimabox/line_country_bot_go/internal/app"
	"github.com/shimabox/line_country_bot_go/internal/country"
)

// fetchDoer sends requests with the Workers fetch API.
type fetchDoer struct {
	client *fetch.Client
}

func (d fetchDoer) Do(r *http.Request) (*http.Response, error) {
	req, err := fetch.NewRequest(r.Context(), r.Method, r.URL.String(), r.Body)
	if err != nil {
		return nil, err
	}
	req.Header = r.Header.Clone()
	return d.client.Do(req, nil)
}

func main() {
	store, err := country.Default()
	if err != nil {
		panic(err)
	}
	workers.Serve(app.NewHandler(app.Deps{
		Getenv:    cloudflare.Getenv,
		Doer:      fetchDoer{client: fetch.NewClient()},
		Countries: store,
	}))
}
