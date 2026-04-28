package contextfiles

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

func Build(paths []string, maxBytes int64) (string, error) {
	var out strings.Builder
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", fmt.Errorf("%s is a directory; pass explicit files", path)
		}
		if info.Size() > maxBytes {
			return "", fmt.Errorf("%s is %d bytes, over limit %d", path, info.Size(), maxBytes)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return "", fmt.Errorf("%s appears to be binary", path)
		}
		fmt.Fprintf(&out, "\n--- File: %s ---\n", path)
		out.Write(data)
		if len(data) == 0 || data[len(data)-1] != '\n' {
			out.WriteByte('\n')
		}
	}
	return strings.TrimLeft(out.String(), "\n"), nil
}
