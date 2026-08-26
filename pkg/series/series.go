// Package series describes how fitsim names and spaces a run of FIT files that
// differ only in when they start. The CLI and the web server both need the rule,
// so it lives here rather than in either of them.
package series

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Step is how much later each extra file in a series starts than the one before
// it.
const Step = time.Minute

// Filename numbers one file of a series: with a count of 3, "run.fit" yields
// run1.fit, run2.fit and run3.fit. A single-file run keeps the exact name it was
// given, so generating one file behaves as it always did.
func Filename(file string, idx, count int) string {
	if count <= 1 {
		return file
	}
	ext := filepath.Ext(file)
	return fmt.Sprintf("%s%d%s", strings.TrimSuffix(file, ext), idx, ext)
}
