package line

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

// SignatureHeader is the request header that carries the webhook signature.
const SignatureHeader = "X-Line-Signature"

// VerifySignature reports whether signature (the Base64 value of the
// x-line-signature header) is the HMAC-SHA256 of the raw request body keyed by
// the channel secret. A missing secret or signature, or a signature that is not
// valid Base64, is treated as invalid.
func VerifySignature(channelSecret string, body []byte, signature string) bool {
	if channelSecret == "" || signature == "" {
		return false
	}
	got, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(channelSecret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}
