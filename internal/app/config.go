package app

import (
	"github.com/shimabox/line_country_bot_go/internal/bot"
	"github.com/shimabox/line_country_bot_go/internal/line"
)

// Names of the secrets and variables read from the environment. Secrets are
// set with `wrangler secret put`; variables live in wrangler.jsonc (or
// .dev.vars for local development).
const (
	EnvChannelSecret      = "CHANNEL_SECRET"
	EnvChannelAccessToken = "CHANNEL_ACCESS_TOKEN"
	EnvWebhookPath        = "WEBHOOK_PATH"
	EnvPublicBaseURL      = "PUBLIC_BASE_URL"
	EnvLineAPIBaseURL     = "LINE_API_BASE_URL"
	EnvProfileImageURL    = "PROFILE_IMAGE_URL"
	EnvProfileTitle       = "PROFILE_TITLE"
	EnvProfileText        = "PROFILE_TEXT"
	EnvProfileLinkText    = "PROFILE_LINK_TEXT"
	EnvProfileLinkURL     = "PROFILE_LINK_URL"
)

// DefaultWebhookPath is used when WEBHOOK_PATH is empty.
const DefaultWebhookPath = "/webhook"

// Config is the configuration for one request.
type Config struct {
	ChannelSecret      string
	ChannelAccessToken string
	WebhookPath        string
	// PublicBaseURL is the origin that serves the flag images. Empty means
	// "derive from the request host".
	PublicBaseURL  string
	LineAPIBaseURL string
	Profile        bot.Profile
}

// LoadConfig reads the configuration and applies defaults.
func LoadConfig(getenv func(string) string) Config {
	cfg := Config{
		ChannelSecret:      getenv(EnvChannelSecret),
		ChannelAccessToken: getenv(EnvChannelAccessToken),
		WebhookPath:        getenv(EnvWebhookPath),
		PublicBaseURL:      getenv(EnvPublicBaseURL),
		LineAPIBaseURL:     getenv(EnvLineAPIBaseURL),
		Profile: bot.Profile{
			ImageURL: getenv(EnvProfileImageURL),
			Title:    getenv(EnvProfileTitle),
			Text:     getenv(EnvProfileText),
			LinkText: getenv(EnvProfileLinkText),
			LinkURL:  getenv(EnvProfileLinkURL),
		},
	}
	if cfg.WebhookPath == "" {
		cfg.WebhookPath = DefaultWebhookPath
	}
	if cfg.LineAPIBaseURL == "" {
		cfg.LineAPIBaseURL = line.DefaultAPIBaseURL
	}
	return cfg
}
