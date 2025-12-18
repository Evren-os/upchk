# upchk

> Concurrent Arch Linux update checker for official repos and AUR

---

## Features

- **Parallel checks** - queries official repos and AUR simultaneously
- **Smart detection** - auto-finds `paru` or `yay`
- **Clean output** - color-coded results with update counts
- **Respectful** - honors `NO_COLOR` and TTY detection

## Installation

```bash
go build -o upchk && sudo mv upchk /usr/local/bin/
```

## Usage

```bash
upchk
```

**Output examples:**

```
[3] Official updates
linux 6.12.1 -> 6.12.2
mesa 24.2.0 -> 24.2.1
firefox 132.0 -> 133.0

[1] AUR updates
visual-studio-code-bin 1.94 -> 1.95
```

```
✓ System is up to date
```

## Requirements

| Package | Source |
|---------|--------|
| `checkupdates` | `pacman-contrib` |
| `paru` or `yay` | AUR |

## License

MIT