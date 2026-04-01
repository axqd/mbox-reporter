package reporter

import (
	"fmt"
	"io"
	"mime"
	"net/mail"
	"sort"
	"strings"

	"github.com/axqd/mbox-reporter/internal/analyzer"
	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
)

const topN = 20

var (
	titleStyle  = color.New(color.FgCyan, color.Bold)
	headerStyle = color.New(color.FgYellow)
	sepStyle    = color.New(color.FgHiBlack)
	nameStyle   = color.New(color.FgWhite, color.Bold)
	numStyle    = color.New(color.FgGreen)
	sizeStyle   = color.New(color.FgMagenta)
	avgStyle    = color.New(color.FgHiBlue)

	dataStyles = []*color.Color{nameStyle, numStyle, sizeStyle, avgStyle, numStyle, sizeStyle, avgStyle, sizeStyle, avgStyle}
)

// Report writes a formatted analysis report to w.
func Report(w io.Writer, stats *analyzer.Stats) {
	printSection(w, "By Email", stats.ByEmail)
	printSection(w, "By Sender Name", stats.BySenderName)
	printSection(w, "By Domain", stats.ByDomain)
	printSection(w, "By Base Domain", stats.ByBaseDomain)
	printMailingListSection(w, "By Mailing List", stats.ByMailingList)
}

type entry struct {
	key  string
	stat *analyzer.SenderStat
}

func avgSize(total int64, count int) string {
	if count == 0 {
		return formatSize(0)
	}
	return formatSize(total / int64(count))
}

func printTitle(w io.Writer, title string) {
	_, _ = fmt.Fprintln(w)
	_, _ = titleStyle.Fprintf(w, "=== %s ===", title)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w)
}

func makeHeaderRow(headers []string) row {
	headerColors := make([]*color.Color, len(headers))
	for i := range headerColors {
		headerColors[i] = headerStyle
	}
	return makeRow(headers, headerColors)
}

func senderStatPlain(key string, s *analyzer.SenderStat) []string {
	return []string{
		key,
		fmt.Sprintf("%d", s.Count),
		formatSize(s.BodySize),
		avgSize(s.BodySize, s.Count),
		fmt.Sprintf("%d", s.AttachmentCount),
		formatSize(s.AttachmentSize),
		avgSize(s.AttachmentSize, s.AttachmentCount),
		formatSize(s.TotalSize),
		avgSize(s.TotalSize, s.Count),
	}
}

// row holds plain text values for width calculation and styled values for output.
type row struct {
	plain  []string
	styled []string
}

func makeRow(plain []string, styles []*color.Color) row {
	styled := make([]string, len(plain))
	for i, p := range plain {
		styled[i] = styles[i].Sprint(p)
	}
	return row{plain: plain, styled: styled}
}

func printTable(w io.Writer, header row, rows []row) {
	// Compute column widths from display width (handles CJK double-width).
	widths := make([]int, len(header.plain))
	for i, h := range header.plain {
		widths[i] = runewidth.StringWidth(h)
	}
	for _, r := range rows {
		for i, cell := range r.plain {
			if cw := runewidth.StringWidth(cell); cw > widths[i] {
				widths[i] = cw
			}
		}
	}

	printRow := func(r row) {
		for i, cell := range r.styled {
			if i > 0 {
				_, _ = fmt.Fprint(w, "  ")
			}
			pad := widths[i] - runewidth.StringWidth(r.plain[i])
			_, _ = fmt.Fprint(w, cell)
			if i < len(r.styled)-1 && pad > 0 {
				_, _ = fmt.Fprint(w, strings.Repeat(" ", pad))
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// Header.
	printRow(header)

	// Separator.
	sepParts := make([]string, len(widths))
	for i, w := range widths {
		sepParts[i] = strings.Repeat("-", w)
	}
	sepRow := row{plain: sepParts, styled: make([]string, len(sepParts))}
	for i, p := range sepParts {
		sepRow.styled[i] = sepStyle.Sprint(p)
	}
	printRow(sepRow)

	// Data rows.
	for _, r := range rows {
		printRow(r)
	}
}

func printSection(w io.Writer, title string, m map[string]*analyzer.SenderStat) {
	if len(m) == 0 {
		return
	}

	entries := make([]entry, 0, len(m))
	for k, v := range m {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].stat.TotalSize > entries[j].stat.TotalSize
	})
	if len(entries) > topN {
		entries = entries[:topN]
	}

	printTitle(w, title)

	headers := []string{"Name", "Count", "Body Size", "Avg Body", "Attach #", "Attach Size", "Avg Attach", "Total Size", "Avg Total"}
	rows := make([]row, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, makeRow(senderStatPlain(e.key, e.stat), dataStyles))
	}

	printTable(w, makeHeaderRow(headers), rows)
}

type mlEntry struct {
	key  string
	stat *analyzer.MailingListStat
}

func printMailingListSection(w io.Writer, title string, m map[string]*analyzer.MailingListStat) {
	if len(m) == 0 {
		return
	}

	entries := make([]mlEntry, 0, len(m))
	for k, v := range m {
		entries = append(entries, mlEntry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].stat.TotalSize > entries[j].stat.TotalSize
	})
	if len(entries) > topN {
		entries = entries[:topN]
	}

	printTitle(w, title)

	headers := []string{"List-Id / Example Sender", "Count", "Body Size", "Avg Body", "Attach #", "Attach Size", "Avg Attach", "Total Size", "Avg Total"}
	senderStyle := color.New(color.FgHiBlack)
	noColor := color.New()

	rows := make([]row, 0, len(entries)*2)
	for _, e := range entries {
		rows = append(rows, makeRow(senderStatPlain(e.key, &e.stat.SenderStat), dataStyles))

		// Second line: example sender, remaining columns empty.
		senderPlain := make([]string, len(headers))
		senderPlain[0] = formatSender(e.stat.ExampleSender)
		senderStyles := make([]*color.Color, len(headers))
		senderStyles[0] = senderStyle
		for i := 1; i < len(senderStyles); i++ {
			senderStyles[i] = noColor
		}
		rows = append(rows, makeRow(senderPlain, senderStyles))
	}

	printTable(w, makeHeaderRow(headers), rows)
}

var mimeDecoder = &mime.WordDecoder{}

func formatSender(raw string) string {
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		// Try MIME decoding the raw value at least.
		if decoded, err := mimeDecoder.DecodeHeader(raw); err == nil {
			return decoded
		}
		return raw
	}
	name := addr.Name
	if decoded, err := mimeDecoder.DecodeHeader(name); err == nil {
		name = decoded
	}
	if name == "" {
		return addr.Address
	}
	return fmt.Sprintf("%s <%s>", name, addr.Address)
}

func formatSize(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
