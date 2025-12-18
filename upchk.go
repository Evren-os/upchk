package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const cmdTimeout = 30 * time.Second

var (
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	reset  = "\033[0m"
)

type result struct {
	source  string
	updates []string
	err     error
}

func init() {
	if os.Getenv("NO_COLOR") != "" || !isTTY() {
		red, green, yellow, cyan, reset = "", "", "", "", ""
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s%v%s\n", red, err, reset)
		os.Exit(1)
	}
}

func run() error {
	if _, err := exec.LookPath("checkupdates"); err != nil {
		return errors.New("checkupdates not found (install pacman-contrib)")
	}

	aurHelper := findAURHelper()
	if aurHelper == "" {
		return errors.New("no AUR helper found (install paru or yay)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*cmdTimeout)
	defer cancel()

	results := make(chan result, 2)

	go checkOfficial(ctx, results)
	go checkAUR(ctx, aurHelper, results)

	var hasOutput bool
	var errs []error

	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		if len(r.updates) > 0 {
			printUpdates(r.source, r.updates)
			hasOutput = true
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	if !hasOutput {
		fmt.Printf("%s✓ All patched. The universe is in balance%s\n", green, reset)
	}

	return nil
}

func findAURHelper() string {
	for _, h := range []string{"paru", "yay"} {
		if _, err := exec.LookPath(h); err == nil {
			return h
		}
	}
	return ""
}

func checkOfficial(ctx context.Context, out chan<- result) {
	updates, err := execCmd(ctx, "checkupdates")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
			out <- result{source: "official"}
			return
		}
		out <- result{source: "official", err: fmt.Errorf("checkupdates: %w", err)}
		return
	}
	out <- result{source: "official", updates: parseLines(updates)}
}

func checkAUR(ctx context.Context, helper string, out chan<- result) {
	updates, err := execCmd(ctx, helper, "-Qua")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			out <- result{source: "aur"}
			return
		}
		out <- result{source: "aur", err: fmt.Errorf("%s: %w", helper, err)}
		return
	}
	out <- result{source: "aur", updates: filterAUR(parseLines(updates))}
}

func execCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func parseLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, strings.Join(strings.Fields(line), " "))
		}
	}
	return lines
}

func filterAUR(lines []string) []string {
	filtered := lines[:0]
	for _, line := range lines {
		if !strings.HasSuffix(line, "[ignored]") {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

func printUpdates(source string, updates []string) {
	count := len(updates)
	switch source {
	case "official":
		fmt.Printf("%s[%s%d%s] The mothership is hailing:%s\n", green, cyan, count, green, reset)
	case "aur":
		fmt.Printf("%s[%s%d%s] New AUR bounties:%s\n", yellow, cyan, count, yellow, reset)
	}
	for _, u := range updates {
		fmt.Println(u)
	}
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
