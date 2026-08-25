package dashboard

import "regexp"

type redactionRule struct {
	pattern     *regexp.Regexp
	replacement string
}

var redactionRules = []redactionRule{
	// JSON loggers commonly quote both the header name and value. Handle that
	// shape before the line-oriented header rule so an escaped quote cannot
	// terminate the credential early.
	{regexp.MustCompile(`(?i)("(?:authorization|proxy-authorization)"\s*:\s*)"(?:\\.|[^"\\\r\n])*"`), `${1}"[REDACTED]"`},
	{regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)[^\r\n]*`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)\b(bearer)(\s+)(?:"(?:\\.|[^"\\\r\n])*"|'[^'\r\n]*'|[A-Za-z0-9._~+/=-]+)`), `${1}${2}[REDACTED]`},
	{regexp.MustCompile(`(?i)\b(basic)(\s+)(?:"(?:\\.|[^"\\\r\n])*"|'[^'\r\n]*'|[A-Za-z0-9+/=]+)`), `${1}${2}[REDACTED]`},
	{regexp.MustCompile(`\b[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{10,}\b`), `[REDACTED_JWT]`},
	{regexp.MustCompile(`(?i)("(?:password|passwd|pwd)"\s*:\s*)"(?:\\.|[^"\\\r\n])*"`), `${1}"[REDACTED]"`},
	{regexp.MustCompile(`(?i)\b(password|passwd|pwd)(\s*[:=]\s*)(?:"(?:\\.|[^"\\\r\n])*"|'[^'\r\n]*'|[^\s,;]+)`), `${1}${2}[REDACTED]`},
	{regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*:\\/\\/[^\s/:@]+:)[^\s/@]+(@)`), `${1}[REDACTED]${2}`},
	{regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^\s/:@]+:)[^\s/@]+(@)`), `${1}[REDACTED]${2}`},
	{regexp.MustCompile(`(?i)\b(cookie|set-cookie|x-api-key|api-key|proxy-authorization)\s*[:=]\s*[^\r\n]+`), `${1}: [REDACTED]`},
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9][A-Z0-9 -]*PRIVATE KEY(?: BLOCK)?-----.*?(?:-----END [A-Z0-9][A-Z0-9 -]*PRIVATE KEY(?: BLOCK)?-----|$)`), `[REDACTED_PRIVATE_KEY]`},
	{regexp.MustCompile(`(?s)-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----.*?(?:-----END (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----|$)`), `[REDACTED_PRIVATE_KEY]`},
	{regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{30,}|sk-[A-Za-z0-9_-]{20,}|xox[baprs]-[A-Za-z0-9-]{10,})\b`), `[REDACTED_TOKEN]`},
}

var (
	privateKeyBeginLine = regexp.MustCompile(`-----BEGIN [A-Z0-9][A-Z0-9 -]*PRIVATE KEY(?: BLOCK)?-----`)
	privateKeyEndLine   = regexp.MustCompile(`-----END [A-Z0-9][A-Z0-9 -]*PRIVATE KEY(?: BLOCK)?-----`)
)

// redactPrivateKeyLogLine carries PEM state across bounded log lines. Redact
// still handles a complete or truncated block supplied as one string; this
// stateful layer prevents a match inside a multi-line key body from exposing
// material after the scanner has split the container stream into lines.
func redactPrivateKeyLogLine(value []byte, withinBlock *bool) (string, bool) {
	if withinBlock == nil {
		return string(value), false
	}
	private := *withinBlock || privateKeyBeginLine.Match(value)
	if !private {
		return string(value), false
	}
	*withinBlock = !privateKeyEndLine.Match(value)
	return "[REDACTED_PRIVATE_KEY]", true
}

// Redact covers the minimum sensitive classes from the threat model. It is a
// defense in depth measure, not a claim that arbitrary secrets are detectable.
func Redact(value string) (string, bool) {
	redacted := false
	for _, rule := range redactionRules {
		if rule.pattern.MatchString(value) {
			value = rule.pattern.ReplaceAllString(value, rule.replacement)
			redacted = true
		}
	}
	return value, redacted
}
