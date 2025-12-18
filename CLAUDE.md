# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
# Build the binary
go build -o upchk

# Run directly
go run upchk.go

# Install to system path
go build -o upchk && sudo mv upchk /usr/local/bin/
```

## Project Overview

upchk is a single-file Go CLI utility for Arch Linux that checks for package updates from both official repositories and the AUR concurrently.

**Core functionality:**
- Uses `checkupdates` (from pacman-contrib) for official repo updates
- Uses `paru` or `yay` with `-Qua` flag for AUR updates
- Runs both checks in parallel via goroutines with channels
- 30-second timeout per command, 60-second total context timeout

**Key behaviors:**
- Exit code 2 from `checkupdates` means no updates (not an error)
- Exit code 1 from AUR helpers means no updates (not an error)
- Filters out `[ignored]` packages from AUR results
- Respects `NO_COLOR` environment variable and auto-detects TTY for color output
- Prints results immediately as each check completes (true concurrent output)
- Single "up to date" message when both sources have no updates
