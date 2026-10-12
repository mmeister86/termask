//go:build !unix

package history

// lock is a no-op fallback where syscall.Flock is unavailable.
func (s Store) lock() (unlock func(), err error) {
	return func() {}, nil
}
