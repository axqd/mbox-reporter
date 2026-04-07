package trasher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strconv"
	"strings"

	emailpkg "github.com/axqd/mbox-reporter/internal/email"
	"github.com/axqd/mbox-reporter/internal/gmail"
	"github.com/axqd/mbox-reporter/internal/mbox"
	"github.com/schollz/progressbar/v3"
)

// GmailClient abstracts the Gmail API operations needed for trashing.
type GmailClient interface {
	TrashThread(ctx context.Context, threadID string) error
}

// ScanResult holds the results of scanning an MBOX file.
type ScanResult struct {
	ThreadIDs    []string // unique Gmail thread IDs (hex)
	MessageCount int      // total messages matching criterion
	TotalSize    int64    // total raw size of matching messages
}

// Trasher orchestrates scanning an MBOX file and trashing matching threads.
type Trasher struct {
	Client      GmailClient
	Criterion   Criterion
	SkipConfirm bool
	RateLimit   int
	Out         io.Writer // user-facing output (os.Stderr)
	In          io.Reader // confirmation input (os.Stdin)
}

// Scan reads an MBOX file and collects thread IDs for messages matching the criterion.
func (t *Trasher) Scan(reader io.Reader, fileSize int64) (*ScanResult, error) {
	bar := progressbar.DefaultBytes(fileSize, "Scanning")
	teeReader := io.TeeReader(reader, bar)

	parser := mbox.NewParser(teeReader)
	threadSet := make(map[string]struct{})
	var messageCount int
	var totalSize int64

	buf := &bytes.Buffer{}
	for {
		buf.Reset()
		err := parser.Next(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = bar.Finish()
			return nil, fmt.Errorf("parse mbox: %w", err)
		}

		raw := buf.Bytes()
		msg, err := mail.ReadMessage(bytes.NewReader(raw))
		if err != nil {
			continue
		}

		parsed := emailpkg.ParseFrom(msg.Header.Get("From"))
		if !t.Criterion.Match(parsed.Address) {
			continue
		}

		thrid := msg.Header.Get("X-GM-THRID")
		if thrid == "" {
			continue
		}

		threadID, err := decimalToHex(strings.TrimSpace(thrid))
		if err != nil {
			continue
		}

		messageCount++
		totalSize += int64(len(raw))
		threadSet[threadID] = struct{}{}
	}

	_ = bar.Finish()

	ids := make([]string, 0, len(threadSet))
	for id := range threadSet {
		ids = append(ids, id)
	}

	return &ScanResult{
		ThreadIDs:    ids,
		MessageCount: messageCount,
		TotalSize:    totalSize,
	}, nil
}

// Run executes the full scan → stats → confirm → trash flow.
func (t *Trasher) Run(ctx context.Context, reader io.Reader, fileSize int64) error {
	result, err := t.Scan(reader, fileSize)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(t.Out)
	t.printStats(result)

	if len(result.ThreadIDs) == 0 {
		_, _ = fmt.Fprintln(t.Out, "Nothing to trash.")
		return nil
	}

	if !t.SkipConfirm {
		_, _ = fmt.Fprintf(t.Out, "\nMove %d threads to trash? [y/N]: ", len(result.ThreadIDs))
		var answer string
		_, _ = fmt.Fscanln(t.In, &answer)
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			_, _ = fmt.Fprintln(t.Out, "Cancelled.")
			return nil
		}
	}

	return t.trash(ctx, result.ThreadIDs)
}

func (t *Trasher) printStats(result *ScanResult) {
	_, _ = fmt.Fprintf(t.Out, "Matching: %s\n", t.Criterion.Description())
	_, _ = fmt.Fprintf(t.Out, "Messages: %d\n", result.MessageCount)
	_, _ = fmt.Fprintf(t.Out, "Threads:  %d\n", len(result.ThreadIDs))
	_, _ = fmt.Fprintf(t.Out, "Size:     %s\n", formatSize(result.TotalSize))
}

func (t *Trasher) trash(ctx context.Context, threadIDs []string) error {
	total := len(threadIDs)
	bar := progressbar.NewOptions(total,
		progressbar.OptionSetDescription(
			fmt.Sprintf("Trashing (rate: %d/sec, 10 quota units/call)", t.RateLimit),
		),
		progressbar.OptionSetWriter(t.Out),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
	)

	var trashed, notFound int
	for _, id := range threadIDs {
		select {
		case <-ctx.Done():
			_ = bar.Finish()
			_, _ = fmt.Fprintf(t.Out, "\nInterrupted. Trashed %d/%d threads.\n", trashed, total)
			return ctx.Err()
		default:
		}

		err := t.Client.TrashThread(ctx, id)
		if errors.Is(err, gmail.ErrNotFound) {
			notFound++
			_ = bar.Add(1)
			continue
		}
		if err != nil {
			_ = bar.Finish()
			_, _ = fmt.Fprintf(t.Out, "\nError after trashing %d/%d threads: %v\n", trashed, total, err)
			return err
		}
		trashed++
		_ = bar.Add(1)
	}

	_ = bar.Finish()
	_, _ = fmt.Fprintf(t.Out, "\nDone. Moved %d threads to trash.", trashed)
	if notFound > 0 {
		_, _ = fmt.Fprintf(t.Out, " Skipped %d threads (not found).", notFound)
	}

	_, _ = fmt.Fprintln(t.Out)

	return nil
}

func decimalToHex(decimal string) (string, error) {
	n, err := strconv.ParseUint(decimal, 10, 64)
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(n, 16), nil
}

func formatSize(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
