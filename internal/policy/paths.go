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

func containsLocalPath(content string) (bool, string) {
	for _, pattern := range localPathPatterns {
		if strings.Contains(content, pattern) {
			return true, pattern
		}
	}
	return false, ""
}
