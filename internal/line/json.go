package line

// A minimal JSON writer for the request bodies this package sends.
//
// The Worker runs every request in a new wasm instance, so encoding/json would
// build its reflection-based encoders again on every request; for the reply
// body that was the largest part of the handler's CPU time. The bodies are
// written by hand instead. The output matches encoding/json byte for byte,
// which the tests check against encoding/json for strings and against a
// reference encoder for every message type.

import "unicode/utf8"

const hexDigits = "0123456789abcdef"

// replacementChar is U+FFFD in UTF-8.
const replacementChar = "\xef\xbf\xbd"

// appendJSONString appends s as a JSON string, escaped as encoding/json does:
// '"' and '\\' and the control characters \b \f \n \r \t get short escapes,
// the other control characters and '<', '>', '&', U+2028 and U+2029 are
// written as \u escapes, and each byte of invalid UTF-8 is replaced with
// U+FFFD (unescaped). DEL and valid non-ASCII characters are copied as is.
func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch c {
			case '"', '\\':
				dst = append(dst, '\\', c)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			dst = append(dst, s[start:i]...)
			dst = append(dst, replacementChar...)
		case r == 0x2028 || r == 0x2029:
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hexDigits[r&0xf])
		default:
			i += size
			continue
		}
		i += size
		start = i
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

// appendArray appends s as a JSON array, writing each element with
// appendElem. A nil slice is written as null, as encoding/json does.
func appendArray[E any](dst []byte, s []E, appendElem func([]byte, E) []byte) []byte {
	if s == nil {
		return append(dst, "null"...)
	}
	dst = append(dst, '[')
	for i, e := range s {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendElem(dst, e)
	}
	return append(dst, ']')
}
