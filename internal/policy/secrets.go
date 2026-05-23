package policy

import (
	"regexp"
	"strings"
)

// secretPrefixes are literal substrings that uniquely identify a known
// secret format. Membership in this set is sufficient evidence.
var secretPrefixes = []struct {
	needle string
	label  string
}{
	{"sk_live_", "stripe live key"},
	{"sk_test_", "stripe test key"},
	{"rk_live_", "stripe restricted key"},
	{"ghp_", "github personal token"},
	{"gho_", "github oauth token"},
	{"ghu_", "github user token"},
	{"ghs_", "github server token"},
	{"xoxb-", "slack bot token"},
	{"xoxp-", "slack user token"},
	{"hooks.slack.com/services/T", "slack webhook"},
	{"AMAZONS3ACCESSKEY", "aws s3 literal"},
	{"-----BEGIN PRIVATE KEY-----", "pem private key"},
	{"-----BEGIN RSA PRIVATE KEY-----", "pem rsa private key"},
}

var (
	awsAccessKeyRe  = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	googleAPIKeyRe  = regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`)
	// Matches scheme://user:password@... for the common credential-bearing
	// URI forms. Requires both a colon and an @ after the scheme.
	credURIRe = regexp.MustCompile(`(?i)\b(mongodb|postgres|postgresql|mysql|redis)(\+srv)?:\/\/[^\s/@]*:[^\s/@]+@`)
)

// containsSecret reports whether content matches a known secret pattern.
// Returns a short label describing the match.
func containsSecret(content string) (bool, string) {
	for _, p := range secretPrefixes {
		if strings.Contains(content, p.needle) {
			return true, p.label
		}
	}

	if m := awsAccessKeyRe.FindString(content); m != "" {
		return true, "aws access key"
	}
	if m := googleAPIKeyRe.FindString(content); m != "" {
		return true, "google api key"
	}
	if m := credURIRe.FindStringSubmatch(content); m != nil {
		return true, m[1] + ":// with credentials"
	}

	return false, ""
}
