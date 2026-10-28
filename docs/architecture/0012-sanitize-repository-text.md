# 12. Treat repository text as untrusted

- Status: Accepted
- Date: 2026-10-28

## Context

Branch names, commit subjects, file paths, diff contents and remote error
output are written by other people. They can carry ANSI and OSC escape
sequences that move the cursor, clear the screen or change the terminal's
title and clipboard, and bidi overrides or zero-width characters that make
a name read differently from what it is. The user decides whether to pull
or delete based on these strings.

## Decision

- Every repository-derived string passes through `sanitize` before it is
  measured or rendered: escape sequences are stripped, then every control
  character and every Unicode format, line separator and paragraph
  separator character (`Cf`, `Zl`, `Zp`) is dropped.
- Diff lines keep their tabs through sanitizing, expand them to four-column
  tab stops, and are then cut to the pane width; nothing wraps.
- Widths are display widths (`x/ansi`), so wide and combining characters
  cannot break the borders.
- The restore command after a deletion is not sanitized, because it must
  restore the exact name; it is built as printable ASCII instead
  (decision 5).

## Consequences

- No repository can drive the terminal or disguise a name.
- Such characters disappear from the display; for example an emoji joined
  with a zero-width joiner shows as its parts.
- Guarded by `internal/ui/sanitize_test.go`, the diff tests and the resize
  sweep with wide text.
