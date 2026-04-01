package reporter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/axqd/mbox-reporter/internal/analyzer"
	"github.com/fatih/color"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		got := formatSize(tt.input)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAvgSize(t *testing.T) {
	if got := avgSize(2048, 2); got != "1.0 KB" {
		t.Errorf("avgSize(2048, 2) = %q, want %q", got, "1.0 KB")
	}
	if got := avgSize(0, 0); got != "0 B" {
		t.Errorf("avgSize(0, 0) = %q, want %q", got, "0 B")
	}
}

func TestFormatSender(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain address",
			input: "alice@example.com",
			want:  "alice@example.com",
		},
		{
			name:  "name and address",
			input: "Alice Smith <alice@example.com>",
			want:  "Alice Smith <alice@example.com>",
		},
		{
			name:  "unparseable",
			input: "just a name",
			want:  "just a name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSender(tt.input)
			if got != tt.want {
				t.Errorf("formatSender(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestReport_WritesOutput(t *testing.T) {
	// Disable color for deterministic output.
	color.NoColor = true
	defer func() { color.NoColor = false }()

	stats := &analyzer.Stats{
		BySenderName: map[string]*analyzer.SenderStat{
			"Alice": {Count: 2, BodySize: 1024, TotalSize: 1024},
		},
		ByEmail: map[string]*analyzer.SenderStat{
			"alice@example.com": {Count: 2, BodySize: 1024, TotalSize: 1024},
		},
		ByDomain: map[string]*analyzer.SenderStat{
			"example.com": {Count: 2, BodySize: 1024, TotalSize: 1024},
		},
		ByBaseDomain: map[string]*analyzer.SenderStat{
			"example.com": {Count: 2, BodySize: 1024, TotalSize: 1024},
		},
		ByMailingList: map[string]*analyzer.MailingListStat{},
	}

	var buf bytes.Buffer
	Report(&buf, stats)
	output := buf.String()

	for _, section := range []string{"By Sender Name", "By Email", "By Domain", "By Base Domain"} {
		if !strings.Contains(output, section) {
			t.Errorf("output missing section %q", section)
		}
	}
	if !strings.Contains(output, "Alice") {
		t.Error("output missing sender name 'Alice'")
	}
	if !strings.Contains(output, "1.0 KB") {
		t.Error("output missing formatted size '1.0 KB'")
	}
}

func TestReport_EmptyStats(t *testing.T) {
	stats := &analyzer.Stats{
		BySenderName:  map[string]*analyzer.SenderStat{},
		ByEmail:       map[string]*analyzer.SenderStat{},
		ByDomain:      map[string]*analyzer.SenderStat{},
		ByBaseDomain:  map[string]*analyzer.SenderStat{},
		ByMailingList: map[string]*analyzer.MailingListStat{},
	}

	var buf bytes.Buffer
	Report(&buf, stats)
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty stats, got: %s", buf.String())
	}
}

func TestReport_MailingListSection(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	stats := &analyzer.Stats{
		BySenderName: map[string]*analyzer.SenderStat{},
		ByEmail:      map[string]*analyzer.SenderStat{},
		ByDomain:     map[string]*analyzer.SenderStat{},
		ByBaseDomain: map[string]*analyzer.SenderStat{},
		ByMailingList: map[string]*analyzer.MailingListStat{
			"list.example.com": {
				SenderStat:    analyzer.SenderStat{Count: 5, BodySize: 2048, TotalSize: 2048},
				ExampleSender: "Alice <alice@example.com>",
			},
		},
	}

	var buf bytes.Buffer
	Report(&buf, stats)
	output := buf.String()

	if !strings.Contains(output, "By Mailing List") {
		t.Error("output missing 'By Mailing List' section")
	}
	if !strings.Contains(output, "list.example.com") {
		t.Error("output missing list ID")
	}
}
