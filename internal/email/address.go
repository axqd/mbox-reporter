package email

import (
	"mime"
	"net/mail"
	"strings"
)

// ParsedSender holds a parsed sender name and email address.
type ParsedSender struct {
	Name    string
	Address string
}

// ParseFrom parses a raw RFC 2822 From header into a sender name and email.
// It handles MIME-encoded words and malformed headers gracefully.
// Returns a ParsedSender with whatever fields could be extracted.
func ParseFrom(rawFrom string) ParsedSender {
	// Decode RFC 2047 MIME encoded words before parsing.
	dec := new(mime.WordDecoder)
	decoded := rawFrom
	if d, err := dec.DecodeHeader(rawFrom); err == nil {
		decoded = d
	}

	addr, err := mail.ParseAddress(decoded)
	if err != nil {
		addr = extractAddressFallback(decoded)
	}

	if addr != nil {
		name := addr.Name
		if name == "" {
			name = addr.Address
		}
		return ParsedSender{Name: name, Address: addr.Address}
	}

	// No parseable email address at all (e.g. From: "Vodafone").
	return ParsedSender{Name: strings.TrimSpace(decoded)}
}

// extractAddressFallback tries to pull an email address from a From header
// that mail.ParseAddress cannot handle (e.g. missing space before '<',
// trailing whitespace inside '<>', or no email at all).
// Returns nil when no email address can be found.
func extractAddressFallback(from string) *mail.Address {
	start := strings.LastIndex(from, "<")
	end := strings.LastIndex(from, ">")
	if start < 0 || end <= start {
		// No angle brackets — try the whole string as a bare address.
		trimmed := strings.TrimSpace(from)
		if strings.Contains(trimmed, "@") {
			return &mail.Address{Address: trimmed}
		}
		return nil
	}
	email := strings.TrimSpace(from[start+1 : end])
	name := strings.TrimSpace(from[:start])
	if !strings.Contains(email, "@") {
		return nil
	}
	// Decode MIME encoded words in the name portion if present.
	dec := new(mime.WordDecoder)
	if decoded, err := dec.DecodeHeader(name); err == nil {
		name = decoded
	}
	return &mail.Address{Name: name, Address: email}
}
