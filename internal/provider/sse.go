package provider

import (
	"bufio"
	"io"
	"strings"
)

// ScanSSE scans a Server-Sent Events stream, invoking onData for each
// `data: ` payload. A `[DONE]` payload terminates the stream without error.
// Lines without the `data: ` prefix (comments, event names) are skipped.
func ScanSSE(r io.Reader, onData func(data string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		if err := onData(data); err != nil {
			return err
		}
	}
	return scanner.Err()
}
