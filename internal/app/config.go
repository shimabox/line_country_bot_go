package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
	EnvReplyDelayMS       = "REPLY_DELAY_MS"
)

// DefaultWebhookPath is used when WEBHOOK_PATH is empty.
const DefaultWebhookPath = "/webhook"

// DefaultReplyDelay is used when REPLY_DELAY_MS is empty or invalid.
// MaxReplyDelay caps it: LINE counts a webhook as timed out when the bot does
// not answer within 2 seconds, and the wait happens before the answer, so the
// wait, both API calls and the handling must fit in that time.
const (
	DefaultReplyDelay = 1000 * time.Millisecond
	MaxReplyDelay     = 1200 * time.Millisecond
)

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
	// ReplyDelay is how long the loading animation is shown before the
	// reply. 0 turns off both the animation and the wait.
	ReplyDelay time.Duration
	// ReplyDelayWarning tells why REPLY_DELAY_MS was not used as given. It
	// is nil when the value was used.
	ReplyDelayWarning error
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
	cfg.ReplyDelay, cfg.ReplyDelayWarning = parseReplyDelay(getenv(EnvReplyDelayMS))
	return cfg
}

// parseReplyDelay reads REPLY_DELAY_MS, a whole number of milliseconds. An
// empty value selects the default; an invalid one selects the default with a
// warning, and one above the limit is capped with a warning.
func parseReplyDelay(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultReplyDelay, nil
	}
	ms, err := strconv.Atoi(s)
	if err != nil || ms < 0 {
		return DefaultReplyDelay, fmt.Errorf("%s=%q is not a non-negative whole number of milliseconds; using %d",
			EnvReplyDelayMS, s, DefaultReplyDelay.Milliseconds())
	}
	if int64(ms) <= MaxReplyDelay.Milliseconds() {
		return time.Duration(ms) * time.Millisecond, nil
	}
	return MaxReplyDelay, fmt.Errorf("%s=%d exceeds the limit of %d; using %d",
		EnvReplyDelayMS, ms, MaxReplyDelay.Milliseconds(), MaxReplyDelay.Milliseconds())
}
