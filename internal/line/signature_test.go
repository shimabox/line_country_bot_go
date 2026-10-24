package line

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	const secret = "test-channel-secret"
	body := []byte(`{"destination":"U0","events":[]}`)
	valid := sign(secret, body)

	tests := []struct {
		name      string
		secret    string
		body      []byte
		signature string
		want      bool
	}{
		{name: "valid", secret: secret, body: body, signature: valid, want: true},
		{name: "tampered body", secret: secret, body: []byte(`{"destination":"U1","events":[]}`), signature: valid, want: false},
		{name: "missing header", secret: secret, body: body, signature: "", want: false},
		{name: "invalid base64", secret: secret, body: body, signature: "!!!not-base64!!!", want: false},
		{name: "different secret", secret: "another-secret", body: body, signature: valid, want: false},
		{name: "empty body valid", secret: secret, body: []byte{}, signature: sign(secret, []byte{}), want: true},
		{name: "empty body wrong signature", secret: secret, body: []byte{}, signature: valid, want: false},
		{name: "empty secret", secret: "", body: body, signature: sign("", body), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifySignature(tt.secret, tt.body, tt.signature); got != tt.want {
				t.Errorf("VerifySignature() = %v, want %v", got, tt.want)
			}
		})
	}
}
