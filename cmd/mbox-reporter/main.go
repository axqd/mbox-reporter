package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/axqd/mbox-reporter/internal/analyzer"
	"github.com/axqd/mbox-reporter/internal/gmail"
	"github.com/axqd/mbox-reporter/internal/mbox"
	"github.com/axqd/mbox-reporter/internal/reporter"
	"github.com/axqd/mbox-reporter/internal/trasher"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "mbox-reporter",
		Usage: "Analyze and manage emails from mbox files",
		Commands: []*cli.Command{
			reportCommand(),
			trashCommand(),
		},
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func showParentHelp(_ context.Context, cmd *cli.Command, _ string) {
	cli.HelpPrinter(cmd.Root().Writer, cli.CommandHelpTemplate, cmd)
}

func reportCommand() *cli.Command {
	var filePath string
	return &cli.Command{
		Name:            "report",
		Usage:           "Analyze an mbox file and output a size report",
		UsageText:       "mbox-reporter report <file>",
		HideHelpCommand: true,
		CommandNotFound: showParentHelp,
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "file",
				UsageText:   "path to the mbox file",
				Destination: &filePath,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if filePath == "" {
				return fmt.Errorf("mbox file path is required")
			}

			file, size, err := openMbox(filePath)
			if err != nil {
				return err
			}
			defer func() { _ = file.Close() }()
			reader, markBarFinish := attachProgressBar(file, size)

			parser := mbox.NewParser(reader)
			stats, err := analyzer.Analyze(parser)
			markBarFinish()
			_, _ = fmt.Fprintln(os.Stderr)
			if err != nil {
				return fmt.Errorf("analyze: %w", err)
			}

			reporter.Report(os.Stdout, stats)
			return nil
		},
	}
}

func trashCommand() *cli.Command {
	var (
		filePath     string
		from         string
		clientSecret string
		yes          bool
		rateLimit    int
	)
	return &cli.Command{
		Name:            "trash",
		Usage:           "Move Gmail threads to trash based on mbox analysis",
		UsageText:       "mbox-reporter trash <file> [options]",
		HideHelpCommand: true,
		CommandNotFound: showParentHelp,
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "file",
				UsageText:   "path to the mbox file (from Google Takeout)",
				Destination: &filePath,
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "from",
				Usage:       "full email address to match sender",
				Destination: &from,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "client-secret",
				Usage:       "path to OAuth2 client_secret.json",
				Destination: &clientSecret,
				Sources:     cli.EnvVars("MBOX_REPORTER_CLIENT_SECRET"),
			},
			&cli.BoolFlag{
				Name:        "yes",
				Aliases:     []string{"y"},
				Usage:       "skip confirmation prompt",
				Destination: &yes,
			},
			&cli.IntFlag{
				Name:        "rate",
				Usage:       "max API calls per second",
				Value:       25,
				Destination: &rateLimit,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if filePath == "" {
				return fmt.Errorf("mbox file path is required")
			}
			if clientSecret == "" {
				return fmt.Errorf("--client-secret flag or MBOX_REPORTER_CLIENT_SECRET env var is required")
			}

			return runTrash(ctx, trashOptions{
				mboxPath:     filePath,
				from:         from,
				clientSecret: clientSecret,
				yes:          yes,
				rateLimit:    rateLimit,
			})
		},
	}
}

type trashOptions struct {
	mboxPath     string
	from         string
	clientSecret string
	yes          bool
	rateLimit    int
}

func runTrash(ctx context.Context, opts trashOptions) error {
	// Open MBOX file.
	file, size, err := openMbox(opts.mboxPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	// Build criterion.
	criterion := trasher.FromAddress{Address: opts.from}

	// Authenticate and create Gmail client.
	svc, err := gmail.NewService(ctx, opts.clientSecret)
	if err != nil {
		return fmt.Errorf("gmail auth: %w", err)
	}
	client := gmail.NewClient(svc, opts.rateLimit)

	// Run the trash flow.
	tr := &trasher.Trasher{
		Client:      client,
		Criterion:   criterion,
		SkipConfirm: opts.yes,
		RateLimit:   opts.rateLimit,
		Out:         os.Stderr,
		In:          os.Stdin,
	}

	err = tr.Run(ctx, file, size)

	// Report backoff hint if applicable.
	if backoffs := client.BackoffCount(); backoffs > 0 {
		suggestedRate := max(opts.rateLimit/2, 1)
		_, _ = fmt.Fprintf(
			os.Stderr, "Hint: %d backoffs occurred. Try --rate=%d next time.\n", backoffs, suggestedRate)
	}

	return err
}

func openMbox(path string) (*os.File, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("open mbox file: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, fmt.Errorf("stat mbox file: %w", err)
	}

	return f, info.Size(), nil
}

func attachProgressBar(reader io.Reader, size int64) (io.Reader, func()) {
	bar := progressbar.DefaultBytes(size, "Analyzing")
	return io.TeeReader(reader, bar), func() {
		_ = bar.Finish()
	}
}
