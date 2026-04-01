package analyzer

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/axqd/mbox-reporter/internal/email"
)

// mockSource implements MessageSource for testing.
type mockSource struct {
	messages [][]byte
	index    int
}

func (m *mockSource) Next(buf *bytes.Buffer) error {
	if m.index >= len(m.messages) {
		return io.EOF
	}
	buf.Write(m.messages[m.index])
	m.index++
	return nil
}

func TestParseFrom(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantName  string
		wantEmail string
	}{
		{
			name:      "MIME name glued to angle bracket",
			input:     "招商银行信用卡中心<ccsvc@message.cmbchina.com>",
			wantName:  "招商银行信用卡中心",
			wantEmail: "ccsvc@message.cmbchina.com",
		},
		{
			name:      "bare address as display name",
			input:     "acctopup@ezlink.com.sg<acctopup@ezlink.com.sg>",
			wantName:  "acctopup@ezlink.com.sg",
			wantEmail: "acctopup@ezlink.com.sg",
		},
		{
			name:      "trailing whitespace inside brackets",
			input:     `"InfoQ" <newsletter@mailer.infoq.com >`,
			wantName:  `"InfoQ"`,
			wantEmail: "newsletter@mailer.infoq.com",
		},
		{
			name:      "decoded MIME with email",
			input:     "又拍网<sender@send.yupoo.com>",
			wantName:  "又拍网",
			wantEmail: "sender@send.yupoo.com",
		},
		{
			name:     "no email at all - Vodafone",
			input:    "Vodafone",
			wantName: "Vodafone",
		},
		{
			name:     "no email at all - quoted",
			input:    `"Singtel"`,
			wantName: `"Singtel"`,
		},
		{
			name:  "empty string",
			input: "",
		},
		{
			name:      "bare email without brackets",
			input:     "user@example.com",
			wantName:  "user@example.com",
			wantEmail: "user@example.com",
		},
		{
			name:     "angle brackets but no @",
			input:    "Name <notanemail>",
			wantName: "Name <notanemail>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := email.ParseFrom(tt.input)
			if got.Name != tt.wantName {
				t.Errorf("name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Address != tt.wantEmail {
				t.Errorf("email = %q, want %q", got.Address, tt.wantEmail)
			}
		})
	}
}

func TestExtractListID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My List <list.example.com>", "list.example.com"},
		{"<only.brackets>", "only.brackets"},
		{"bare.list.id", "bare.list.id"},
		{"  spaced  ", "spaced"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractListID(tt.input)
			if got != tt.want {
				t.Errorf("extractListID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// makeRawMessage constructs a minimal RFC 2822 message from headers and body.
func makeRawMessage(from, subject, body string, extraHeaders ...string) []byte {
	var msg strings.Builder
	_, _ = fmt.Fprintf(&msg, "From: %s\r\nSubject: %s\r\n", from, subject)
	for _, h := range extraHeaders {
		msg.WriteString(h + "\r\n")
	}
	msg.WriteString("\r\n" + body)
	return []byte(msg.String())
}

func TestAnalyzeMessage(t *testing.T) {
	raw := makeRawMessage("Alice Smith <alice@example.com>", "Test", "Hello world")
	res, err := analyzeMessage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.senderName != "Alice Smith" {
		t.Errorf("senderName = %q, want %q", res.senderName, "Alice Smith")
	}
	if res.email != "alice@example.com" {
		t.Errorf("email = %q, want %q", res.email, "alice@example.com")
	}
	if res.domain != "example.com" {
		t.Errorf("domain = %q, want %q", res.domain, "example.com")
	}
	if res.bodySize != int64(len("Hello world")) {
		t.Errorf("bodySize = %d, want %d", res.bodySize, len("Hello world"))
	}
}

func TestAnalyzeMessage_NoName(t *testing.T) {
	raw := makeRawMessage("bob@example.com", "Test", "body")
	res, err := analyzeMessage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When no display name, senderName falls back to the email address.
	if res.senderName != "bob@example.com" {
		t.Errorf("senderName = %q, want %q", res.senderName, "bob@example.com")
	}
}

func TestAnalyzeMessage_Multipart(t *testing.T) {
	boundary := "----boundary123"
	body := fmt.Sprintf(
		"--%s\r\nContent-Type: text/plain\r\n\r\nHello\r\n"+
			"--%s\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"doc.pdf\"\r\n\r\nPDFDATA\r\n"+
			"--%s--\r\n",
		boundary, boundary, boundary,
	)
	raw := makeRawMessage(
		"Alice <alice@example.com>", "With attachment", "",
		fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"", boundary),
	)
	// Replace the empty body with the multipart body.
	raw = append(raw, []byte(body)...)

	res, err := analyzeMessage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.attachmentCount != 1 {
		t.Errorf("attachmentCount = %d, want 1", res.attachmentCount)
	}
	if res.bodySize == 0 {
		t.Error("expected non-zero bodySize for text part")
	}
	if res.attachmentSize == 0 {
		t.Error("expected non-zero attachmentSize for attachment part")
	}
}

func TestAnalyzeMessage_ListId(t *testing.T) {
	raw := makeRawMessage(
		"Alice <alice@example.com>", "List mail", "body",
		"List-Id: My List <mylist.example.com>",
	)
	res, err := analyzeMessage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.mailingList != "mylist.example.com" {
		t.Errorf("mailingList = %q, want %q", res.mailingList, "mylist.example.com")
	}
}

func TestAnalyze_Integration(t *testing.T) {
	src := &mockSource{
		messages: [][]byte{
			makeRawMessage("Alice <alice@example.com>", "Msg 1", "Hello"),
			makeRawMessage("Alice <alice@example.com>", "Msg 2", "World"),
			makeRawMessage("Bob <bob@other.org>", "Msg 3", "Hi"),
		},
	}

	stats, err := Analyze(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check aggregation by email.
	aliceStat, ok := stats.ByEmail["alice@example.com"]
	if !ok {
		t.Fatal("expected stats for alice@example.com")
	}
	if aliceStat.Count != 2 {
		t.Errorf("alice count = %d, want 2", aliceStat.Count)
	}

	bobStat, ok := stats.ByEmail["bob@other.org"]
	if !ok {
		t.Fatal("expected stats for bob@other.org")
	}
	if bobStat.Count != 1 {
		t.Errorf("bob count = %d, want 1", bobStat.Count)
	}

	// Check by domain.
	if _, ok := stats.ByDomain["example.com"]; !ok {
		t.Error("expected domain stats for example.com")
	}
	if _, ok := stats.ByDomain["other.org"]; !ok {
		t.Error("expected domain stats for other.org")
	}
}

func TestAnalyze_EmptySource(t *testing.T) {
	src := &mockSource{}
	stats, err := Analyze(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats.ByEmail) != 0 {
		t.Errorf("expected empty ByEmail, got %d entries", len(stats.ByEmail))
	}
}

func TestAnalyzeMessage_FallbackFrom(t *testing.T) {
	// From header that mail.ParseAddress can't handle (no space before <).
	raw := makeRawMessage("又拍网<sender@send.yupoo.com>", "Test", "body")
	res, err := analyzeMessage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.email != "sender@send.yupoo.com" {
		t.Errorf("email = %q, want %q", res.email, "sender@send.yupoo.com")
	}
}

func TestAnalyzeMessage_NoEmailFrom(t *testing.T) {
	raw := makeRawMessage("Vodafone", "Alert", "body")
	res, err := analyzeMessage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When no email can be parsed, senderName is the raw header text.
	if res.senderName != "Vodafone" {
		t.Errorf("senderName = %q, want %q", res.senderName, "Vodafone")
	}
	if res.email != "" {
		t.Errorf("email = %q, want empty", res.email)
	}
}

func TestAddStat(t *testing.T) {
	m := make(map[string]*SenderStat)
	addStat(m, "key1", 100, 1, 50, 150)
	addStat(m, "key1", 200, 2, 100, 300)

	s := m["key1"]
	if s.Count != 2 {
		t.Errorf("count = %d, want 2", s.Count)
	}
	if s.BodySize != 300 {
		t.Errorf("bodySize = %d, want 300", s.BodySize)
	}
	if s.AttachmentCount != 3 {
		t.Errorf("attachmentCount = %d, want 3", s.AttachmentCount)
	}
	if s.AttachmentSize != 150 {
		t.Errorf("attachmentSize = %d, want 150", s.AttachmentSize)
	}
	if s.TotalSize != 450 {
		t.Errorf("totalSize = %d, want 450", s.TotalSize)
	}
}
