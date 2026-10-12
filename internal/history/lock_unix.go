//go:build unix

package history

import (
	"os"
	"path/filepath"
	"syscall"
)

// lock acquires an exclusive advisory lock on s.path+".lock" for the whole
// List→prune→writeAll sequence, so concurrent processes cannot interleave
// reads and writes and lose sessions.
func (s Store) lock() (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
