package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	version        = "1.0.0"
	commandTimeout = 30 * time.Second
)

var (
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorReset  = "\033[0m"
)

type updateResult struct {
	output string
	err    error
}

func init() {
	if os.Getenv("NO_COLOR") != "" || !isTerminal(os.Stdout) {
		colorRed = ""
		colorGreen = ""
		colorYellow = ""
		colorCyan = ""
		colorReset = ""
	}
}

func main() {
	noVersion := flag.Bool("no-ver", false, "Strip version details from output")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("update-checker v%s\n", version)
		os.Exit(0)
	}

	if _, err := exec.LookPath("checkupdates"); err != nil {
		fmt.Fprintf(os.Stderr, "%scheckupdates is MIA. Install 'pacman-contrib' or rot.%s\n", colorRed, colorReset)
		os.Exit(1)
	}

	aurHelper := detectAURHelper()
	if aurHelper == "" {
		fmt.Fprintf(os.Stderr, "%sNo AUR helper found. Install paru or yay.%s\n", colorRed, colorReset)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*commandTimeout)
	defer cancel()

	var wg sync.WaitGroup
	officialChan := make(chan updateResult)
	aurChan := make(chan updateResult)

	wg.Add(2)

	go func() {
		defer wg.Done()
		output, err := fetchOfficialUpdates(ctx)
		officialChan <- updateResult{output, err}
	}()

	go func() {
		defer wg.Done()
		output, err := fetchAURUpdates(ctx, aurHelper)
		aurChan <- updateResult{output, err}
	}()

	go func() {
		wg.Wait()
		close(officialChan)
		close(aurChan)
	}()

	officialResult := <-officialChan
	aurResult := <-aurChan

	if officialResult.err != nil {
		fmt.Fprintf(os.Stderr, "%sFailed to check official updates: %v%s\n", colorRed, officialResult.err, colorReset)
		os.Exit(1)
	}

	if aurResult.err != nil {
		fmt.Fprintf(os.Stderr, "%sFailed to check AUR updates: %v%s\n", colorRed, aurResult.err, colorReset)
		os.Exit(1)
	}

	officialUpdates := officialResult.output
	aurUpdates := aurResult.output

	if *noVersion {
		officialUpdates = stripVersions(officialUpdates)
		aurUpdates = stripVersions(aurUpdates)
	}

	displayResults(officialUpdates, aurUpdates)
}

func detectAURHelper() string {
	helpers := []string{"paru", "yay"}
	for _, helper := range helpers {
		if _, err := exec.LookPath(helper); err == nil {
			return helper
		}
	}
	return ""
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after %v", commandTimeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%w: %s", err, string(exitErr.Stderr))
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func fetchOfficialUpdates(ctx context.Context) (string, error) {
	output, err := runCommand(ctx, "checkupdates")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
			return "", nil
		}
		return "", fmt.Errorf("checkupdates failed: %w", err)
	}
	return output, nil
}

func fetchAURUpdates(ctx context.Context, aurHelper string) (string, error) {
	output, err := runCommand(ctx, aurHelper, "-Qua")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", nil
		}
		return "", fmt.Errorf("%s failed: %w", aurHelper, err)
	}
	if output == "" {
		return "", nil
	}

	var builder strings.Builder
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.Join(strings.Fields(line), " ")
		if !strings.HasSuffix(line, "[ignored]") {
			if builder.Len() > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(line)
		}
	}
	return builder.String(), nil
}

func stripVersions(updates string) string {
	if updates == "" {
		return ""
	}
	var builder strings.Builder
	for _, line := range strings.Split(updates, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) > 0 {
			if builder.Len() > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(parts[0])
		}
	}
	return builder.String()
}

func countUpdates(updates string) int {
	if updates == "" {
		return 0
	}
	count := 0
	for _, line := range strings.Split(updates, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func displayResults(official, aur string) {
	officialCount := countUpdates(official)
	aurCount := countUpdates(aur)

	if officialCount == 0 && aurCount == 0 {
		fmt.Printf("%sAll patched. The universe is in balance.%s\n", colorGreen, colorReset)
		return
	}

	if officialCount > 0 {
		fmt.Printf("%sThe mothership is hailing: %s%d%s new directives.%s\n",
			colorGreen, colorCyan, officialCount, colorGreen, colorReset)
		fmt.Println(official)
	} else {
		fmt.Printf("%sMainline is stable. As it should be.%s\n", colorGreen, colorReset)
	}

	if aurCount > 0 {
		fmt.Printf("%s%s%d%s new AUR bounties.%s\n",
			colorYellow, colorCyan, aurCount, colorYellow, colorReset)
		fmt.Println(aur)
	} else {
		fmt.Printf("%sAUR sleeps. Silence is deadly.%s\n", colorGreen, colorReset)
	}
}

func isTerminal(f *os.File) bool {
	if fileInfo, err := f.Stat(); err == nil {
		return (fileInfo.Mode() & os.ModeCharDevice) != 0
	}
	return false
}