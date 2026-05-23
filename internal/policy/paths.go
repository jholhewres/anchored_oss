package policy

import "strings"

var localPathPatterns = []string{
	"/home/",
	"/Users/",
	`C:\Users\`,
	"C:/Users/",
	"~/",
	"/tmp/",
	"/var/folders/",
	"%TEMP%",
	"%TMP%",
	"/var/tmp/",
	"/private/tmp/",
	`C:\Windows\`,
	"C:/Windows/",
}

// ContainsLocalPath reports whether content includes a local filesystem
// pattern (home dirs, temp dirs, Windows system paths). Returns the
// matched pattern when found.
func ContainsLocalPath(content string) (bool, string) {
	for _, pattern := range localPathPatterns {
		if strings.Contains(content, pattern) {
			return true, pattern
		}
	}
	return false, ""
}

// internal alias used by the existing ContentFilter implementation.
func containsLocalPath(content string) (bool, string) {
	return ContainsLocalPath(content)
}
