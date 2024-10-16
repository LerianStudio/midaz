package utils

import "strings"

func Format(commands ...string) string {
	return strings.Join(commands, "\n")
}
