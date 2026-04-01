package trasher

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestFromAddress_Match(t *testing.T) {
	c := FromAddress{Address: "shop@email.stackcommerce.com"}
	if !c.Match("shop@email.stackcommerce.com") {
		t.Error("expected match for exact address")
	}
	if !c.Match("Shop@Email.StackCommerce.com") {
		t.Error("expected case-insensitive match")
	}
	if c.Match("other@example.com") {
		t.Error("expected no match for different address")
	}
	if c.Match("") {
		t.Error("expected no match for empty string")
	}
}

func TestFromAddress_Description(t *testing.T) {
	c := FromAddress{Address: "shop@example.com"}
	want := "messages from shop@example.com"
	if got := c.Description(); got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
}

func TestDecimalToHex(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1672749667872724050", "1736ccf5d7b4d452"},
		{"0", "0"},
		{"255", "ff"},
		{"16", "10"},
	}
	for _, tt := range tests {
		got, err := decimalToHex(tt.input)
		if err != nil {
			t.Errorf("decimalToHex(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("decimalToHex(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}

	if _, err := decimalToHex("notanumber"); err == nil {
		t.Error("expected error for non-numeric input")
	}
}

// mockClient records TrashThread calls.
type mockClient struct {
	trashedIDs []string
	failAfter  int // fail after N successful calls (-1 = never fail)
}

func (m *mockClient) TrashThread(_ context.Context, threadID string) error {
	if m.failAfter >= 0 && len(m.trashedIDs) >= m.failAfter {
		return fmt.Errorf("API error")
	}
	m.trashedIDs = append(m.trashedIDs, threadID)
	return nil
}

// makeMbox builds a minimal MBOX stream with From separators and X-GM-THRID headers.
func makeMbox(messages ...struct {
	from  string
	thrid string
	body  string
}) []byte {
	var buf bytes.Buffer
	for _, m := range messages {
		_, _ = fmt.Fprintf(&buf, "From sender@example.com Mon Jan  1 00:00:00 2024\r\n")
		_, _ = fmt.Fprintf(&buf, "From: %s\r\n", m.from)
		if m.thrid != "" {
			_, _ = fmt.Fprintf(&buf, "X-GM-THRID: %s\r\n", m.thrid)
		}
		_, _ = fmt.Fprintf(&buf, "\r\n%s\r\n", m.body)
	}
	return buf.Bytes()
}

func TestScan(t *testing.T) {
	data := makeMbox(
		struct{ from, thrid, body string }{"Alice <alice@example.com>", "1672749667872724050", "hello"},
		struct{ from, thrid, body string }{"Alice <alice@example.com>", "1672749667872724050", "reply"}, // same thread
		struct{ from, thrid, body string }{"Bob <bob@other.org>", "255", "hi"},
		struct{ from, thrid, body string }{"Alice <alice@example.com>", "16", "another thread"},
	)

	tr := &Trasher{
		Criterion: FromAddress{Address: "alice@example.com"},
		Out:       &bytes.Buffer{},
	}

	result, err := tr.Scan(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if result.MessageCount != 3 {
		t.Errorf("MessageCount = %d, want 3", result.MessageCount)
	}
	// 2 unique threads: 1672749667872724050 and 16
	if len(result.ThreadIDs) != 2 {
		t.Errorf("ThreadIDs count = %d, want 2", len(result.ThreadIDs))
	}
	if result.TotalSize == 0 {
		t.Error("expected non-zero TotalSize")
	}
}

func TestScan_NoMatches(t *testing.T) {
	data := makeMbox(
		struct{ from, thrid, body string }{"Bob <bob@other.org>", "255", "hi"},
	)

	tr := &Trasher{
		Criterion: FromAddress{Address: "alice@example.com"},
		Out:       &bytes.Buffer{},
	}

	result, err := tr.Scan(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if result.MessageCount != 0 {
		t.Errorf("MessageCount = %d, want 0", result.MessageCount)
	}
	if len(result.ThreadIDs) != 0 {
		t.Errorf("ThreadIDs count = %d, want 0", len(result.ThreadIDs))
	}
}

func TestRun_Confirm_Yes(t *testing.T) {
	data := makeMbox(
		struct{ from, thrid, body string }{"Alice <alice@example.com>", "255", "hello"},
	)

	client := &mockClient{failAfter: -1}
	out := &bytes.Buffer{}
	tr := &Trasher{
		Client:    client,
		Criterion: FromAddress{Address: "alice@example.com"},
		RateLimit: 25,
		Out:       out,
		In:        strings.NewReader("y\n"),
	}

	err := tr.Run(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(client.trashedIDs) != 1 {
		t.Errorf("trashed %d threads, want 1", len(client.trashedIDs))
	}
	if client.trashedIDs[0] != "ff" {
		t.Errorf("trashed ID = %q, want %q", client.trashedIDs[0], "ff")
	}
}

func TestRun_Confirm_No(t *testing.T) {
	data := makeMbox(
		struct{ from, thrid, body string }{"Alice <alice@example.com>", "255", "hello"},
	)

	client := &mockClient{failAfter: -1}
	out := &bytes.Buffer{}
	tr := &Trasher{
		Client:    client,
		Criterion: FromAddress{Address: "alice@example.com"},
		Out:       out,
		In:        strings.NewReader("n\n"),
	}

	err := tr.Run(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(client.trashedIDs) != 0 {
		t.Errorf("trashed %d threads, want 0 (cancelled)", len(client.trashedIDs))
	}
	if !strings.Contains(out.String(), "Cancelled") {
		t.Error("expected 'Cancelled' in output")
	}
}

func TestRun_SkipConfirm(t *testing.T) {
	data := makeMbox(
		struct{ from, thrid, body string }{"Alice <alice@example.com>", "255", "hello"},
	)

	client := &mockClient{failAfter: -1}
	out := &bytes.Buffer{}
	tr := &Trasher{
		Client:      client,
		Criterion:   FromAddress{Address: "alice@example.com"},
		SkipConfirm: true,
		RateLimit:   25,
		Out:         out,
		In:          strings.NewReader(""),
	}

	err := tr.Run(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(client.trashedIDs) != 1 {
		t.Errorf("trashed %d threads, want 1", len(client.trashedIDs))
	}
	// Stats should still be shown
	if !strings.Contains(out.String(), "Threads:") {
		t.Error("expected stats in output even with --yes")
	}
}

func TestRun_NoMatches(t *testing.T) {
	data := makeMbox(
		struct{ from, thrid, body string }{"Bob <bob@other.org>", "255", "hi"},
	)

	client := &mockClient{failAfter: -1}
	out := &bytes.Buffer{}
	tr := &Trasher{
		Client:    client,
		Criterion: FromAddress{Address: "alice@example.com"},
		Out:       out,
		In:        strings.NewReader(""),
	}

	err := tr.Run(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if !strings.Contains(out.String(), "Nothing to trash") {
		t.Error("expected 'Nothing to trash' in output")
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
	}
	for _, tt := range tests {
		if got := formatSize(tt.input); got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
