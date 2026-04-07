package analyzer

import (
	"bytes"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/mail"
	"runtime"
	"strings"
	"sync"

	emailpkg "github.com/axqd/mbox-reporter/internal/email"
	"github.com/axqd/mbox-reporter/internal/mbox"
	"golang.org/x/net/publicsuffix"
)

// MessageSource yields raw RFC 2822 message bytes one at a time.
// The caller provides a buffer to write into. Returns io.EOF when
// there are no more messages.
type MessageSource interface {
	Next(buf *bytes.Buffer) error
}

// SenderStat holds aggregated size statistics for a grouping key.
type SenderStat struct {
	Count           int
	BodySize        int64
	AttachmentCount int
	AttachmentSize  int64
	TotalSize       int64
}

// MailingListStat holds stats for a mailing list plus an example sender.
type MailingListStat struct {
	SenderStat
	ExampleSender string // "Name <email>" of the first sender seen
}

// Stats holds all aggregated statistics.
type Stats struct {
	BySenderName  map[string]*SenderStat
	ByEmail       map[string]*SenderStat
	ByDomain      map[string]*SenderStat
	ByBaseDomain  map[string]*SenderStat
	ByMailingList map[string]*MailingListStat
}

type messageResult struct {
	senderName      string
	email           string
	domain          string
	baseDomain      string
	mailingList     string
	fromHeader      string // raw "Name <email>" for mailing list example
	bodySize        int64
	attachmentCount int
	attachmentSize  int64
}

// Analyze reads all messages from the parser and returns aggregated stats.
// Parsing is done sequentially; per-message analysis runs in parallel.
// A bounded buffer pool provides back-pressure and avoids per-message allocation.
// Messages from addresses in excludeEmails are skipped.
func Analyze(src MessageSource, excludeEmails map[string]struct{}) (*Stats, error) {
	numWorkers := runtime.NumCPU()

	const initCap = 64 * 1024     // 64 KB initial buffer capacity
	const shrinkCap = 4 * 1024 * 1024 // 4 MB shrink threshold
	pool := mbox.NewBufferPool(numWorkers, initCap, shrinkCap)

	rawCh := make(chan *bytes.Buffer, numWorkers)
	resultCh := make(chan messageResult, numWorkers)

	// Parser goroutine: read mbox sequentially.
	var parseErr error
	go func() {
		defer close(rawCh)
		for {
			buf := pool.Get()
			err := src.Next(buf)
			if err != nil {
				pool.Put(buf)
				if err != io.EOF {
					parseErr = err
				}
				return
			}
			rawCh <- buf
		}
	}()

	// Worker pool: parse and analyze each message.
	var wg sync.WaitGroup
	for range numWorkers {
		wg.Go(func() {
			for buf := range rawCh {
				res, err := analyzeMessage(buf.Bytes())
				pool.Put(buf)
				if err != nil {
					continue
				}
				if excludeEmails != nil {
					if _, excluded := excludeEmails[strings.ToLower(res.email)]; excluded {
						continue
					}
				}
				resultCh <- res
			}
		})
	}

	// Close results channel when all workers are done.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collector: merge results into stats.
	stats := &Stats{
		BySenderName:  make(map[string]*SenderStat),
		ByEmail:       make(map[string]*SenderStat),
		ByDomain:      make(map[string]*SenderStat),
		ByBaseDomain:  make(map[string]*SenderStat),
		ByMailingList: make(map[string]*MailingListStat),
	}

	for res := range resultCh {
		totalSize := res.bodySize + res.attachmentSize

		addStat(stats.BySenderName, res.senderName, res.bodySize, res.attachmentCount, res.attachmentSize, totalSize)
		addStat(stats.ByEmail, res.email, res.bodySize, res.attachmentCount, res.attachmentSize, totalSize)
		addStat(stats.ByDomain, res.domain, res.bodySize, res.attachmentCount, res.attachmentSize, totalSize)
		addStat(stats.ByBaseDomain, res.baseDomain, res.bodySize, res.attachmentCount, res.attachmentSize, totalSize)
		if res.mailingList != "" {
			ml, ok := stats.ByMailingList[res.mailingList]
			if !ok {
				ml = &MailingListStat{ExampleSender: res.fromHeader}
				stats.ByMailingList[res.mailingList] = ml
			}
			ml.Count++
			ml.BodySize += res.bodySize
			ml.AttachmentCount += res.attachmentCount
			ml.AttachmentSize += res.attachmentSize
			ml.TotalSize += totalSize
		}
	}

	return stats, parseErr
}

func addStat(m map[string]*SenderStat, key string, bodySize int64, attachCount int, attachSize int64, totalSize int64) {
	s, ok := m[key]
	if !ok {
		s = &SenderStat{}
		m[key] = s
	}
	s.Count++
	s.BodySize += bodySize
	s.AttachmentCount += attachCount
	s.AttachmentSize += attachSize
	s.TotalSize += totalSize
}

func analyzeMessage(raw []byte) (messageResult, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return messageResult{}, err
	}

	var res messageResult

	// Parse sender.
	rawFrom := msg.Header.Get("From")
	res.fromHeader = rawFrom

	parsed := emailpkg.ParseFrom(rawFrom)
	res.senderName = parsed.Name
	res.email = parsed.Address
	if at := strings.LastIndex(parsed.Address, "@"); at >= 0 {
		res.domain = parsed.Address[at+1:]
		if bd, err := publicsuffix.EffectiveTLDPlusOne(res.domain); err == nil {
			res.baseDomain = bd
		} else {
			res.baseDomain = res.domain
		}
	}

	// DEBUG: log messages with empty or suspicious domains.
	if res.domain == "" || strings.TrimSpace(res.domain) != res.domain || strings.ContainsAny(res.domain, "\t\r\n\x00") {
		slog.Debug("empty/suspicious domain",
			"message_id", msg.Header.Get("Message-Id"),
			"subject", msg.Header.Get("Subject"),
			"date", msg.Header.Get("Date"),
			"raw_from", msg.Header.Get("From"),
			"decoded_from", parsed.Name,
			"parsed_email", res.email,
			"domain", res.domain,
			"domain_bytes", []byte(res.domain),
		)
	}

	// Mailing list.
	if listID := msg.Header.Get("List-Id"); listID != "" {
		res.mailingList = extractListID(listID)
	}

	// Compute body and attachment sizes.
	contentType := msg.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		// Not multipart: entire body is body size.
		n, _ := io.Copy(io.Discard, msg.Body)
		res.bodySize = n
		return res, nil
	}

	// Walk multipart parts.
	mr := multipart.NewReader(msg.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		n, _ := io.Copy(io.Discard, part)

		disposition := part.Header.Get("Content-Disposition")
		if strings.HasPrefix(disposition, "attachment") {
			res.attachmentCount++
			res.attachmentSize += n
		} else {
			res.bodySize += n
		}
		_ = part.Close()
	}

	return res, nil
}


// extractListID extracts the list identifier from a List-Id header value.
// e.g. "My List <list.example.com>" -> "list.example.com"
func extractListID(value string) string {
	if start := strings.LastIndex(value, "<"); start >= 0 {
		if end := strings.LastIndex(value, ">"); end > start {
			return value[start+1 : end]
		}
	}
	return strings.TrimSpace(value)
}
