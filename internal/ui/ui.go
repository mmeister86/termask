package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
)

var (
	Prompt  = color.New(color.FgCyan, color.Bold)
	Success = color.New(color.FgGreen, color.Bold)
	Warn    = color.New(color.FgYellow)
	Err     = color.New(color.FgRed, color.Bold)
	Dim     = color.New(color.Faint)
	Badge   = color.New(color.FgMagenta)
)

const boxWidth = 60

// PrintHeader prints a styled query box with a provider+model badge.
func PrintHeader(query, providerName, model string) {
	border := strings.Repeat("─", boxWidth)
	badge := fmt.Sprintf("[%s / %s]", providerName, model)

	fmt.Fprintln(os.Stderr)
	Prompt.Fprintf(os.Stderr, "┌%s┐\n", border)

	// Badge line
	pad := boxWidth - len(badge)
	if pad < 1 {
		pad = 1
	}
	Badge.Fprintf(os.Stderr, "│ %s%s│\n", badge, strings.Repeat(" ", pad-1))
	Prompt.Fprintf(os.Stderr, "├%s┤\n", border)

	// Query — word-wrap
	words := strings.Fields(query)
	line := ""
	for _, w := range words {
		if len(line)+len(w)+1 > boxWidth-2 {
			Prompt.Fprintf(os.Stderr, "│ %-*s │\n", boxWidth-2, line)
			line = w
		} else {
			if line != "" {
				line += " "
			}
			line += w
		}
	}
	if line != "" {
		Prompt.Fprintf(os.Stderr, "│ %-*s │\n", boxWidth-2, line)
	}
	Prompt.Fprintf(os.Stderr, "└%s┘\n\n", border)
}

// PrintFooter prints a subtle separator after the answer.
func PrintFooter() {
	fmt.Fprintln(os.Stderr)
	Dim.Fprintln(os.Stderr, strings.Repeat("─", boxWidth+2))
}

// Spinner shows an animated spinner. Call stop() to dismiss it.
func Spinner(msg string) (stop func()) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	done := make(chan struct{})
	go func() {
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", len(msg)+4))
				return
			default:
				Dim.Fprintf(os.Stderr, "\r%s %s ", frames[i%len(frames)], msg)
				time.Sleep(80 * time.Millisecond)
				i++
			}
		}
	}()
	return func() { close(done) }
}
