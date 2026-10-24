// Package app is the HTTP entry point of the bot: routing, webhook
// verification and dispatching events to the bot.
package app

import (
	"errors"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"strings"

	"github.com/shimabox/line_country_bot_go/internal/bot"
	"github.com/shimabox/line_country_bot_go/internal/country"
	"github.com/shimabox/line_country_bot_go/internal/line"
)

const (
	rootBody        = "line_country_bot_go\n"
	maxWebhookBytes = 1 << 20
)

// Deps are the handler's dependencies. Getenv and Doer are the only ways the
// handler reaches the outside world, so tests can replace them.
type Deps struct {
	// Getenv reads a secret or variable. It is called on every request.
	Getenv func(string) string
	// Doer sends reply API requests.
	Doer line.Doer
	// Countries is the country data.
	Countries *country.Store
	// NewRand returns the random source for one request. Optional.
	NewRand func() *rand.Rand
	// Logf writes a log line. Optional; defaults to log.Printf.
	Logf func(format string, args ...any)
}

type handler struct {
	deps Deps
}

// NewHandler returns the bot's http.Handler.
func NewHandler(deps Deps) http.Handler {
	if deps.NewRand == nil {
		deps.NewRand = func() *rand.Rand { return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) }
	}
	if deps.Logf == nil {
		deps.Logf = log.Printf
	}
	return &handler{deps: deps}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cfg := LoadConfig(h.deps.Getenv)
	switch r.URL.Path {
	case cfg.WebhookPath:
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		h.webhook(w, r, cfg)
	case "/":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, rootBody)
	default:
		http.NotFound(w, r)
	}
}

func (h *handler) webhook(w http.ResponseWriter, r *http.Request, cfg Config) {
	if cfg.ChannelSecret == "" {
		h.deps.Logf("config error: %s is not set", EnvChannelSecret)
		http.Error(w, "Server misconfigured.", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBytes+1))
	if err != nil {
		http.Error(w, "Cannot read body.", http.StatusBadRequest)
		return
	}
	if len(body) > maxWebhookBytes {
		http.Error(w, "Request body too large.", http.StatusRequestEntityTooLarge)
		return
	}
	if !line.VerifySignature(cfg.ChannelSecret, body, r.Header.Get(line.SignatureHeader)) {
		http.Error(w, "Invalid signature.", http.StatusBadRequest)
		return
	}
	req, err := line.ParseWebhook(body)
	if err != nil {
		http.Error(w, "Invalid request body.", http.StatusBadRequest)
		return
	}

	if err := cfg.Profile.Validate(); err != nil {
		h.deps.Logf("config warning: %v", err)
	}
	if cfg.ChannelAccessToken == "" && len(req.Events) > 0 {
		h.deps.Logf("config error: %s is not set; replies will fail", EnvChannelAccessToken)
	}
	opts := bot.Options{FlagBaseURL: publicBaseURL(cfg, r), Profile: cfg.Profile}
	b := bot.New(h.deps.Countries, h.deps.NewRand())
	client := line.NewClient(cfg.LineAPIBaseURL, cfg.ChannelAccessToken, h.deps.Doer)

	// Events are handled in order and independently: a failed reply is logged
	// and does not stop the others. LINE gets 200 either way, since a retry
	// cannot reuse the reply token.
	for _, ev := range req.Events {
		reply := b.Respond(ev, opts)
		if len(reply.Messages) == 0 || ev.ReplyToken == "" {
			h.deps.Logf("event=%s kind=%s code=%s replied=false", ev.Type, reply.Kind, reply.Code)
			continue
		}
		if err := client.Reply(r.Context(), ev.ReplyToken, reply.Messages); err != nil {
			status, class := classifyReplyError(err)
			h.deps.Logf("event=%s kind=%s code=%s replied=false status=%d error=%s", ev.Type, reply.Kind, reply.Code, status, class)
			continue
		}
		h.deps.Logf("event=%s kind=%s code=%s replied=true messages=%d", ev.Type, reply.Kind, reply.Code, len(reply.Messages))
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, "OK")
}

// Classes of reply failures written to the log.
const (
	errorReplyAPI   = "reply_api_error"
	errorTransport  = "transport_error"
	errorBadRequest = "request_error"
)

// classifyReplyError returns the reply API status (0 when no response
// arrived) and a fixed class for a failed reply. The log gets only these: the
// error text may carry the API's response body.
func classifyReplyError(err error) (int, string) {
	var apiErr *line.APIError
	switch {
	case errors.As(err, &apiErr):
		return apiErr.StatusCode, errorReplyAPI
	case errors.Is(err, line.ErrSend):
		return 0, errorTransport
	default:
		return 0, errorBadRequest
	}
}

// publicBaseURL returns PUBLIC_BASE_URL, or https://{Host} when it is unset.
func publicBaseURL(cfg Config, r *http.Request) string {
	if cfg.PublicBaseURL != "" {
		return strings.TrimRight(cfg.PublicBaseURL, "/")
	}
	return "https://" + r.Host
}
