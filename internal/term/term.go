// Package term holds small, dependency-free terminal helpers.
package term

import "os"

// IsTerminal reports whether f is a character device such as a console.
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
