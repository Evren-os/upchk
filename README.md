# upchk

Personal Arch update checker I threw together for my CachyOS setup. Checks official repos and AUR concurrently because waiting is boring.

## What it does

- Runs `checkupdates` and `paru`/`yay -Qua` in parallel
- Shows update counts with color-coded output
- Strips version numbers if you want (`--no-ver`)
- Respects `NO_COLOR` and auto-detects TTY

## Install
```bash
go build -o upchk
sudo mv upchk /usr/local/bin/
```

## Usage
```bash
# Check for updates
upchk

# Without version details
upchk --no-ver

# Show version
upchk --version
```

## Requirements

- `checkupdates` (from `pacman-contrib`)
- `paru` or `yay`

## Why this exists

Got tired of running two commands. Made one command. That's it.

## License

Do whatever you want with it.

## License

MIT License - see [LICENSE](LICENSE) file for details.