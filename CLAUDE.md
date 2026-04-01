# CLAUDE.md

## Purpose

Go CLI tool that analyzes Google Takeout MBOX files for email statistics, and trashes matching Gmail threads using embedded thread IDs.

## User-facing docs

See [README.md](README.md) for usage, installation, and Gmail API setup.

## Architecture

### Shared

- `internal/mbox` — streams RFC 2822 messages from MBOX files
- `internal/email` — parses sender info from From headers
- `openMbox()` in main — shared file open/stat helper

### Report: MBOX analysis

```
CLI Input -> MBOX Parser -> Analyzer -> Reporter -> CLI Output
```

- `internal/analyzer` — aggregates size statistics by sender name, email, domain, base domain, mailing list, and attachments
- `internal/reporter` — formats and writes the report to stdout

### Trash: Gmail batch trash via MBOX thread IDs

```
CLI Input -> MBOX Parser -> Criterion Filter -> X-GM-THRID extraction -> Gmail API (threads.trash)
```

- `internal/trasher` — scans MBOX, filters by criterion (extensible interface), collects unique thread IDs
- `internal/gmail` — OAuth2 auth, rate-limited `threads.trash` with exponential backoff

## Development

- Always run `make` to ensure lint, test, and build can pass after code changes
- Keep code changes and newly added features minimal
- Use `errors.Is` for sentinel error checks and `errors.AsType` for typed error checks