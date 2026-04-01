package mbox

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// Parser reads a mbox file and yields individual raw message bytes.
type Parser struct {
	reader  *bufio.Reader
	started bool
}

// NewParser creates a new streaming mbox parser.
func NewParser(r io.Reader) *Parser {
	return &Parser{reader: bufio.NewReader(r)}
}

// Next writes the raw bytes of the next message into buf.
// The caller must provide a buffer (typically from a BufferPool).
// Returns io.EOF when there are no more messages.
func (p *Parser) Next(buf *bytes.Buffer) error {
	for {
		line, err := p.reader.ReadSlice('\n')

		if errors.Is(err, bufio.ErrBufferFull) {
			// Line longer than bufio's internal buffer; consume it in chunks.
			if len(line) > 0 && !p.started {
				// Not yet inside a message — skip preamble data.
			} else if len(line) > 0 && p.started {
				buf.Write(line)
			}
			// Keep reading the rest of this long line.
			for errors.Is(err, bufio.ErrBufferFull) {
				line, err = p.reader.ReadSlice('\n')
				if len(line) > 0 && p.started {
					buf.Write(line)
				}
			}
			if err != nil && err != io.EOF {
				if buf.Len() > 0 {
					return nil
				}
				return err
			}
			continue
		}

		if len(line) > 0 {
			if isFromLine(line) {
				if !p.started {
					p.started = true
				} else if buf.Len() > 0 {
					return nil
				}
			} else if p.started {
				buf.Write(line)
			}
		}

		if err != nil {
			if buf.Len() > 0 {
				return nil
			}
			return err
		}
	}
}

// isFromLine reports whether a line is a mbox "From " separator.
func isFromLine(line []byte) bool {
	return len(line) >= 5 && line[0] == 'F' && line[1] == 'r' && line[2] == 'o' && line[3] == 'm' && line[4] == ' '
}
