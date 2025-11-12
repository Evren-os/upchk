package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

const (
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorReset  = "\033[0m"
)

// Constants for yt-dlp arguments and settings.
const (
	defaultFilenamePattern = "%(title)s [%(id)s][%(height)sp][%(fps)sfps][%(vcodec)s][%(acodec)s].%(ext)s"
	defaultMergeFormat     = "mkv"
	codecAV1               = "av1"
	codecVP9               = "vp9"

	// Settings for social media compatibility (optimized for modern platforms).
	socmFormat      = "bv*[vcodec^=avc][height<=1080]+ba[acodec^=mp4a]/b[vcodec^=avc][height<=1080]"
	socmMergeFormat = "mp4"
)

// fatalf prints a formatted error message to stderr and exits with status 1.
func fatalf(format string, args ...interface{}) {
	errorMessage := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%sError: %s%s\n", colorRed, errorMessage, colorReset)
	os.Exit(1)
}

// checkDependencies ensures that all required command-line tools are installed and in the PATH.
func checkDependencies(cmds ...string) {
	for _, cmd := range cmds {
		if _, err := exec.LookPath(cmd); err != nil {
			fatalf("%s is not installed or not found in PATH", cmd)
		}
	}
}

// validateURL performs basic URL validation.
func validateURL(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false
	}
	_, err := url.Parse(rawURL)
	return err == nil && (strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://"))
}

// sanitizeAndDeduplicateURLs cleans and deduplicates the URL list.
func sanitizeAndDeduplicateURLs(urls []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, rawURL := range urls {
		cleanURL := strings.TrimSpace(rawURL)
		if cleanURL == "" {
			continue
		}
		if !validateURL(cleanURL) {
			fmt.Printf("%sWarning: Skipping invalid URL: %s%s\n", colorYellow, cleanURL, colorReset)
			continue
		}
		if !seen[cleanURL] {
			seen[cleanURL] = true
			result = append(result, cleanURL)
		}
	}

	return result
}

// buildYTDLPArgs constructs the command-line arguments for yt-dlp based on user flags.
func buildYTDLPArgs(url, codecPref, destinationPath, cookiesFrom string, socm bool) []string {
	// Determine output template.
	outputTemplate := defaultFilenamePattern
	if destinationPath != "" {
		if info, err := os.Stat(destinationPath); err == nil && info.IsDir() {
			outputTemplate = filepath.Join(destinationPath, defaultFilenamePattern)
		} else {
			outputTemplate = destinationPath
		}
	}

	// Base arguments.
	args := []string{
		"--remote-components", "ejs:github",
		"--prefer-free-formats",
		"--format-sort-force",
		"--no-mtime",
		"--output", outputTemplate,
		"--external-downloader", "aria2c",
		"--external-downloader-args", "-x 16 -s 32 -k 1M --disk-cache=128M --enable-color=false",
	}

	if cookiesFrom != "" {
		args = append(args, "--cookies-from-browser", cookiesFrom)
	}

	if socm {
		// Social media compatibility settings override others.
		args = append(args,
			"--merge-output-format", socmMergeFormat,
			"--format", socmFormat,
		)
	} else {
		// Standard high-quality download settings.
		maxHeight := 2160
		formatString := fmt.Sprintf("bv*[height<=%d]+ba/bv*[height<=%d]", maxHeight, maxHeight)

		var sortString string
		switch strings.ToLower(codecPref) {
		case codecAV1:
			sortString = "res,fps,vcodec:av01,vcodec:vp9.2,vcodec:vp9,vcodec:hev1,acodec:opus"
		case codecVP9:
			sortString = "res,fps,vcodec:vp9,vcodec:vp9.2,vcodec:av01,vcodec:hev1,acodec:opus"
		default:
			fatalf("Invalid codec preference. Use '%s' or '%s'.", codecAV1, codecVP9)
		}

		args = append(args,
			"--merge-output-format", defaultMergeFormat,
			"--format", formatString,
			"--format-sort", sortString,
		)
	}

	// Finally, add the URL.
	args = append(args, url)
	return args
}

// downloadURL executes yt-dlp for a single URL in a goroutine.
func downloadURL(url, codecPref, destinationPath, cookiesFrom string, socm bool, wg *sync.WaitGroup, sem chan struct{}, failedURLsChan chan<- string) {
	defer wg.Done()
	defer func() { <-sem }() // Release semaphore slot.

	fmt.Printf("Starting download: %s%s%s\n", colorCyan, url, colorReset)

	cmdArgs := buildYTDLPArgs(url, codecPref, destinationPath, cookiesFrom, socm)
	cmd := exec.Command("yt-dlp", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("%sFailed to download: %s (exit code: %v)%s\n", colorRed, url, err, colorReset)
		failedURLsChan <- url
	} else {
		fmt.Printf("%sCompleted download: %s%s\n", colorGreen, url, colorReset)
	}
}

// batchDownload handles downloading multiple URLs concurrently.
func batchDownload(urls []string, codecPref, destinationPath, cookiesFrom string, socm bool, parallel int) {

	// Sanitize and deduplicate URLs
	cleanURLs := sanitizeAndDeduplicateURLs(urls)
	if len(cleanURLs) == 0 {
		fatalf("no valid URLs provided")
	}

	if len(cleanURLs) != len(urls) {
		fmt.Printf("Processing %s%d%s valid URLs (filtered from %s%d%s)\n", colorCyan, len(cleanURLs), colorReset, colorCyan, len(urls), colorReset)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup
	sem := make(chan struct{}, parallel)
	failedURLsChan := make(chan string, len(cleanURLs))
	done := make(chan bool, 1)

	// Launch downloads
	go func() {
		for _, url := range cleanURLs {
			wg.Add(1)
			sem <- struct{}{}
			go downloadURL(url, codecPref, destinationPath, cookiesFrom, socm, &wg, sem, failedURLsChan)
		}
		wg.Wait()
		done <- true
	}()

	// Wait for completion or signal
	select {
	case <-done:
		// Downloads completed normally
	case <-sigChan:
		fmt.Printf("\n%sReceived termination signal. Waiting for active downloads to complete...%s\n", colorYellow, colorReset)
		<-done
	}

	close(failedURLsChan)

	var failedURLs []string
	for url := range failedURLsChan {
		failedURLs = append(failedURLs, url)
	}

	if len(failedURLs) > 0 {
		fmt.Printf("\n--- Summary ---\n")
		fmt.Printf("%s%d/%d downloads failed.%s\n", colorRed, len(failedURLs), len(cleanURLs), colorReset)
		fmt.Println("Failed URLs:")
		for _, url := range failedURLs {
			fmt.Printf("  - %s%s%s\n", colorRed, url, colorReset)
		}
		os.Exit(1)
	} else {
		fmt.Printf("\n--- Summary ---\n")
		fmt.Printf("%sAll %d downloads completed successfully.%s\n", colorGreen, len(cleanURLs), colorReset)
	}
}

func main() {
	// Define command-line flags.
	var (
		codecPref       string
		destinationPath string
		cookiesFrom     string
		socm            bool
		parallel        int
	)

	flag.StringVar(&codecPref, "codec", codecAV1, "Preferred video codec (av1 or vp9). Ignored if -socm is used.")
	flag.StringVar(&destinationPath, "d", "", "Download destination. Can be a directory or a full file path.")
	flag.StringVar(&cookiesFrom, "cookies-from", "", "Load cookies from the specified browser (e.g., firefox, chrome).")
	flag.BoolVar(&socm, "socm", false, "Optimize for social media compatibility (MP4, H.264/AAC).")
	flag.IntVar(&parallel, "p", 4, "Number of parallel downloads for batch mode.")

	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage: ytmax [options] URL [URL...]\n\n")
		fmt.Fprintf(out, "A wrapper for yt-dlp to download single videos or batches with optimized settings.\n")
		fmt.Fprintf(out, "Automatically detects batch mode when multiple URLs are provided.\n\n")
		fmt.Fprintf(out, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(out, "\nExamples:\n")
		fmt.Fprintf(out, "  Single download:\n")
		fmt.Fprintf(out, "    ytmax -codec vp9 -d /videos https://youtu.be/VIDEO_ID\n")
		fmt.Fprintf(out, "  Batch download:\n")
		fmt.Fprintf(out, "    ytmax -d /videos -p 6 \"URL1\" \"URL2\" \"URL3\"\n")
		fmt.Fprintf(out, "    ytmax --cookies-from firefox \"URL1\" \"URL2\"\n")
	}

	flag.Parse()

	// Check for URL arguments.
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	if parallel < 1 {
		fatalf("number of parallel downloads (-p) must be at least 1")
	}

	// Check dependencies early
	checkDependencies("yt-dlp", "aria2c")

	urls := flag.Args()

	// Detect batch mode vs single download.
	if len(urls) == 1 {
		// Single download mode.
		url := strings.TrimSpace(urls[0])
		if !validateURL(url) {
			fatalf("invalid URL provided: %s", url)
		}

		cmdArgs := buildYTDLPArgs(url, codecPref, destinationPath, cookiesFrom, socm)
		cmd := exec.Command("yt-dlp", cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
	} else {
		// Batch download mode.
		batchDownload(urls, codecPref, destinationPath, cookiesFrom, socm, parallel)
	}
}
