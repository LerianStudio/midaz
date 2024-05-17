package console

import (
	"fmt"
	"strings"
)

// DefaultLineSize is the line size used in Title function.
const DefaultLineSize = 80

// Line returns a single line. Eg: -------.
func Line(size int) string {
	return strings.Repeat("-", size)
}

// DoubleLine returns a doubled line. Eg: ========.
func DoubleLine(size int) string {
	return strings.Repeat("=", size)
}

// Title returns a title with double line. Eg: ====== title ======.
func Title(title string) string {
	title = fmt.Sprintf(" %s ", title)
	startIndex := (DefaultLineSize / 2) - (len(title) / 2)
	delta := len(title) % 2

	return fmt.Sprintf("%s%s%s",
		DoubleLine(startIndex),
		title,
		DoubleLine((startIndex)+delta))
}
