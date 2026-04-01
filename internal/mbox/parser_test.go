package mbox

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// helper to call Next with a fresh buffer and return the bytes.
func nextMessage(t *testing.T, p *Parser) ([]byte, error) {
	t.Helper()
	var buf bytes.Buffer
	err := p.Next(&buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func TestNext_SingleMessage(t *testing.T) {
	input := "From sender@example.com Mon Jan 1 00:00:00 2024\nSubject: Hello\n\nBody here\n"
	p := NewParser(strings.NewReader(input))

	raw, err := nextMessage(t, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(raw), "Subject: Hello") {
		t.Errorf("expected message to contain Subject header, got: %s", raw)
	}
	if !strings.Contains(string(raw), "Body here") {
		t.Errorf("expected message to contain body, got: %s", raw)
	}

	_, err = nextMessage(t, p)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got: %v", err)
	}
}

func TestNext_MultipleMessages(t *testing.T) {
	input := "From a@example.com Mon Jan 1 00:00:00 2024\nSubject: First\n\nBody 1\n" +
		"From b@example.com Mon Jan 1 00:00:00 2024\nSubject: Second\n\nBody 2\n"
	p := NewParser(strings.NewReader(input))

	raw1, err := nextMessage(t, p)
	if err != nil {
		t.Fatalf("first message: unexpected error: %v", err)
	}
	if !strings.Contains(string(raw1), "First") {
		t.Errorf("expected first message, got: %s", raw1)
	}

	raw2, err := nextMessage(t, p)
	if err != nil {
		t.Fatalf("second message: unexpected error: %v", err)
	}
	if !strings.Contains(string(raw2), "Second") {
		t.Errorf("expected second message, got: %s", raw2)
	}

	_, err = nextMessage(t, p)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got: %v", err)
	}
}

func TestNext_EmptyInput(t *testing.T) {
	p := NewParser(strings.NewReader(""))
	_, err := nextMessage(t, p)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got: %v", err)
	}
}

func TestNext_NoFromLine(t *testing.T) {
	p := NewParser(strings.NewReader("Subject: Hello\n\nBody\n"))
	_, err := nextMessage(t, p)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got: %v", err)
	}
}

func TestNext_EmptyMessageBetweenFromLines(t *testing.T) {
	input := "From a@example.com Mon Jan 1 00:00:00 2024\n" +
		"From b@example.com Mon Jan 1 00:00:00 2024\n" +
		"Subject: Real\n\nBody\n"
	p := NewParser(strings.NewReader(input))

	raw, err := nextMessage(t, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(raw), "Real") {
		t.Errorf("expected message with 'Real', got: %s", raw)
	}

	_, err = nextMessage(t, p)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got: %v", err)
	}
}

func TestNext_PartialLineAtEOF(t *testing.T) {
	// No trailing newline after body
	input := "From sender@example.com Mon Jan 1 00:00:00 2024\nSubject: Test\n\nPartial"
	p := NewParser(strings.NewReader(input))

	raw, err := nextMessage(t, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(raw), "Partial") {
		t.Errorf("expected partial content, got: %s", raw)
	}
}

func TestNext_FromLineAtEOFWithoutNewline(t *testing.T) {
	// From line without trailing newline should still be recognized.
	input := "garbage\nFrom sender@example.com Mon Jan 1 00:00:00 2024\nSubject: Test\n\nBody"
	p := NewParser(strings.NewReader(input))

	raw, err := nextMessage(t, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(raw), "Body") {
		t.Errorf("expected message body, got: %s", raw)
	}
}

func TestNext_ReadErrorBeforeStarted(t *testing.T) {
	// A non-EOF error before finding a From line should be propagated, not masked as io.EOF.
	errRead := errors.New("disk read error")
	r := &failingReader{
		data: "not a from line\n",
		err:  errRead,
	}
	p := NewParser(r)

	_, err := nextMessage(t, p)
	if !errors.Is(err, errRead) {
		t.Fatalf("expected %v, got: %v", errRead, err)
	}
}

func TestNext_ReadErrorAfterStarted(t *testing.T) {
	// A non-EOF error after accumulating message data should still return the data.
	errRead := errors.New("disk read error")
	r := &failingReader{
		data: "From sender@example.com Mon Jan 1 00:00:00 2024\nSubject: Test\n\nBody",
		err:  errRead,
	}
	p := NewParser(r)

	raw, err := nextMessage(t, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(raw), "Body") {
		t.Errorf("expected message body, got: %s", raw)
	}

	// Next call should surface the read error.
	_, err = nextMessage(t, p)
	if !errors.Is(err, errRead) {
		t.Fatalf("expected %v, got: %v", errRead, err)
	}
}

// failingReader returns data from its buffer, then returns err (instead of io.EOF).
type failingReader struct {
	data string
	err  error
	pos  int
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= len(r.data) {
		return n, r.err
	}
	return n, nil
}

func TestIsFromLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"From sender@example.com Mon Jan 1 00:00:00 2024\n", true},
		{"From someone\n", true},
		{"Subject: From here\n", false},
		{"From", false}, // no space after "From"
		{"", false},
		{"\n", false},
	}
	for _, tt := range tests {
		got := isFromLine([]byte(tt.line))
		if got != tt.want {
			t.Errorf("isFromLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}
