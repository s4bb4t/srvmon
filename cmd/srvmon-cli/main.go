package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	bgRed  = "\033[41m"
	bgGrn  = "\033[42m"
	bgYel  = "\033[43m"
	bgCyn  = "\033[46m"

	clearScreen = "\033[2J\033[H"
	hideCursor  = "\033[?25l"
	showCursor  = "\033[?25h"
	moveHome    = "\033[H"
)

type checkResult struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Error     string `json:"error"`
	Timestamp string `json:"timestamp"`
}

type healthResponse struct {
	Status    string        `json:"status"`
	Version   string        `json:"version"`
	Checks    []checkResult `json:"checks"`
	Timestamp string        `json:"timestamp"`
}

type readinessResponse struct {
	Ready     bool          `json:"ready"`
	Reason    string        `json:"reason"`
	Checks    []checkResult `json:"checks"`
	Timestamp string        `json:"timestamp"`
}

func statusIcon(status string) (string, string) {
	switch status {
	case "STATUS_UP":
		return green + "●" + reset, green + "UP" + reset
	case "STATUS_DOWN":
		return red + "●" + reset, red + "DOWN" + reset
	case "STATUS_DEGRADED":
		return yellow + "●" + reset, yellow + "DEGRADED" + reset
	default:
		return dim + "●" + reset, dim + "UNKNOWN" + reset
	}
}

func statusBadge(status string) string {
	switch status {
	case "STATUS_UP":
		return bgGrn + bold + " UP " + reset
	case "STATUS_DOWN":
		return bgRed + bold + " DOWN " + reset
	case "STATUS_DEGRADED":
		return bgYel + bold + " DEGRADED " + reset
	default:
		return bgCyn + bold + " UNKNOWN " + reset
	}
}

func readyBadge(ready bool) string {
	if ready {
		return bgGrn + bold + " READY " + reset
	}
	return bgRed + bold + " NOT READY " + reset
}

func fetch(url string, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// render builds the full frame into a buffer and returns it as a string.
// Writing the whole frame at once avoids flicker on repaint.
func render(addr string, timeout time.Duration) string {
	var b strings.Builder

	// header
	b.WriteString("\n")
	b.WriteString(bold + cyan + "  srvmon" + reset + dim + " — service health monitor" + reset + "\n")
	b.WriteString(dim + "  " + strings.Repeat("─", 48) + reset + "\n")

	// health
	url := fmt.Sprintf("http://%s/health", addr)
	body, err := fetch(url, timeout)
	if err != nil {
		b.WriteString(fmt.Sprintf("\n  %s  %s\n", red+"●"+reset, "Cannot reach "+bold+addr+reset))
		b.WriteString(fmt.Sprintf("     %s%s%s\n\n", dim, err.Error(), reset))
		b.WriteString(dim + "  retrying..." + reset + "\n")
		return b.String()
	}

	var h healthResponse
	if err := json.Unmarshal(body, &h); err != nil {
		b.WriteString(fmt.Sprintf("\n  %sparse error: %s%s\n", red, err.Error(), reset))
		return b.String()
	}

	b.WriteString(fmt.Sprintf("\n  %s  Health: %s", bold+"HEALTH"+reset, statusBadge(h.Status)))
	if h.Version != "" {
		b.WriteString(fmt.Sprintf("  %s%s%s", dim, h.Version, reset))
	}
	b.WriteString("\n\n")

	if len(h.Checks) > 0 {
		nameW := 0
		for _, c := range h.Checks {
			if len(c.Name) > nameW {
				nameW = len(c.Name)
			}
		}

		for i, c := range h.Checks {
			icon, label := statusIcon(c.Status)
			connector := "├"
			if i == len(h.Checks)-1 {
				connector = "└"
			}
			b.WriteString(fmt.Sprintf("  %s%s──%s %s%-*s%s  %s %s", dim, connector, reset, bold, nameW, c.Name, reset, icon, label))
			if c.Message != "" {
				b.WriteString(fmt.Sprintf("  %s%s%s", dim, c.Message, reset))
			}
			b.WriteString("\n")
			if c.Error != "" {
				padding := "│"
				if i == len(h.Checks)-1 {
					padding = " "
				}
				b.WriteString(fmt.Sprintf("  %s%s%s     %s%s%s\n", dim, padding, reset, red, c.Error, reset))
			}
		}
	}

	// readiness
	rURL := fmt.Sprintf("http://%s/ready", addr)
	rBody, err := fetch(rURL, timeout)
	if err == nil {
		var r readinessResponse
		if err := json.Unmarshal(rBody, &r); err == nil {
			b.WriteString(fmt.Sprintf("\n  %s  Readiness: %s", bold+"READY"+reset, readyBadge(r.Ready)))
			if !r.Ready && r.Reason != "" {
				b.WriteString(fmt.Sprintf("  %s%s%s", dim, r.Reason, reset))
			}
			b.WriteString("\n")
		}
	}

	// footer
	b.WriteString(fmt.Sprintf("\n  %s%s%s\n", dim, time.Now().Format("15:04:05"), reset))

	return b.String()
}

var (
	addr     string
	timeout  time.Duration
	watch    bool
	interval time.Duration
)

func main() {
	root := &cobra.Command{
		Use:   "srvmon-cli",
		Short: "CLI client for srvmon service health monitoring",
		Long:  "Query srvmon HTTP endpoints and display service health and readiness status.",
		RunE: func(cmd *cobra.Command, args []string) error {
			frame := render(addr, timeout)
			if !watch {
				fmt.Print(frame)
				// exit 1 if service is unreachable
				if _, err := fetch(fmt.Sprintf("http://%s/health", addr), timeout); err != nil {
					os.Exit(1)
				}
				return nil
			}

			// watch mode: hide cursor, clear screen, repaint in-place
			fmt.Print(hideCursor + clearScreen)
			defer fmt.Print(showCursor)

			fmt.Print(frame)

			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for range ticker.C {
				frame = render(addr, timeout)
				// move cursor home and overwrite — no flicker, no scroll
				fmt.Print(moveHome + clearScreen + frame)
			}

			return nil
		},
	}

	health := &cobra.Command{
		Use:   "health",
		Short: "Show only health status",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := fetch(fmt.Sprintf("http://%s/health", addr), timeout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s● Cannot reach %s%s\n  %s%s\n", red, addr, reset, err.Error(), reset)
				os.Exit(1)
			}
			var h healthResponse
			if err := json.Unmarshal(body, &h); err != nil {
				return err
			}
			fmt.Print(renderHealth(h))
			return nil
		},
	}

	ready := &cobra.Command{
		Use:   "ready",
		Short: "Show only readiness status",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := fetch(fmt.Sprintf("http://%s/ready", addr), timeout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s● Cannot reach %s%s\n  %s%s\n", red, addr, reset, err.Error(), reset)
				os.Exit(1)
			}
			var r readinessResponse
			if err := json.Unmarshal(body, &r); err != nil {
				return err
			}
			fmt.Print(renderReady(r))
			return nil
		},
	}

	root.PersistentFlags().StringVarP(&addr, "addr", "a", "localhost:8080", "srvmon HTTP address")
	root.PersistentFlags().DurationVarP(&timeout, "timeout", "t", 3*time.Second, "request timeout")
	root.Flags().BoolVarP(&watch, "watch", "w", false, "continuously poll and update in-place")
	root.Flags().DurationVarP(&interval, "interval", "i", 2*time.Second, "poll interval (with --watch)")

	root.AddCommand(health, ready)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func renderHealth(h healthResponse) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n  %s  Health: %s", bold+"HEALTH"+reset, statusBadge(h.Status)))
	if h.Version != "" {
		b.WriteString(fmt.Sprintf("  %s%s%s", dim, h.Version, reset))
	}
	b.WriteString("\n\n")

	if len(h.Checks) > 0 {
		nameW := 0
		for _, c := range h.Checks {
			if len(c.Name) > nameW {
				nameW = len(c.Name)
			}
		}
		for i, c := range h.Checks {
			icon, label := statusIcon(c.Status)
			connector := "├"
			if i == len(h.Checks)-1 {
				connector = "└"
			}
			b.WriteString(fmt.Sprintf("  %s%s──%s %s%-*s%s  %s %s", dim, connector, reset, bold, nameW, c.Name, reset, icon, label))
			if c.Message != "" {
				b.WriteString(fmt.Sprintf("  %s%s%s", dim, c.Message, reset))
			}
			b.WriteString("\n")
			if c.Error != "" {
				padding := "│"
				if i == len(h.Checks)-1 {
					padding = " "
				}
				b.WriteString(fmt.Sprintf("  %s%s%s     %s%s%s\n", dim, padding, reset, red, c.Error, reset))
			}
		}
	}
	b.WriteString("\n")
	return b.String()
}

func renderReady(r readinessResponse) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n  %s  Readiness: %s", bold+"READY"+reset, readyBadge(r.Ready)))
	if !r.Ready && r.Reason != "" {
		b.WriteString(fmt.Sprintf("  %s%s%s", dim, r.Reason, reset))
	}
	b.WriteString("\n\n")
	return b.String()
}
