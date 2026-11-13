//go:build !windows

package term

import "os"

// EnableVT is a no-op outside Windows: Unix terminals interpret ANSI natively.
func EnableVT(*os.File) bool { return true }
