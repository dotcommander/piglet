package safeguard

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// injectionCheck is a single metacharacter injection validator.
type injectionCheck struct {
	name    string
	check   func(cmd string) bool
	message string
}

// injectionChecks are the 7 shell metacharacter injection validators.
// These catch parser-level attacks that regex pattern matching doesn't detect.
// Order: cheapest checks first.
var injectionChecks = []injectionCheck{
	{
		name:    "control-characters",
		check:   hasControlCharacters,
		message: "contains control characters (potential parser confusion)",
	},
	{
		name:    "command-substitution",
		check:   hasCommandSubstitution,
		message: "contains command substitution ($() or backticks) — potential remote code execution",
	},
	{
		name:    "ifs-injection",
		check:   hasIFSInjection,
		message: "contains $IFS variable — potential word-splitting bypass",
	},
	{
		name:    "brace-expansion",
		check:   hasDangerousBraceExpansion,
		message: "contains dangerous brace expansion — potential command evasion",
	},
	{
		name:    "process-substitution",
		check:   hasProcessSubstitution,
		message: "contains process substitution (<() or >()) — potential file descriptor injection",
	},
	{
		name:    "backslash-operators",
		check:   hasBackslashOperators,
		message: "contains backslash-escaped shell operators — potential parser differential attack",
	},
	{
		name:    "variable-redirect",
		check:   hasVariableRedirect,
		message: "contains variable expansion adjacent to redirect — potential config injection",
	},
}

// ValidateInjection runs all injection checks against a bash command.
// Returns nil if safe, an error describing the violation if blocked.
func ValidateInjection(cmd string) error {
	for _, ic := range injectionChecks {
		if ic.check(cmd) {
			return fmt.Errorf("injection blocked (%s): %s", ic.name, ic.message)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Check implementations
// ---------------------------------------------------------------------------

// hasControlCharacters detects non-printable characters (0x00-0x1F) except
// tab (0x09), newline (0x0A), and carriage return (0x0D) which are normal
// in shell commands.
func hasControlCharacters(cmd string) bool {
	for _, r := range cmd {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
	}
	return false
}

// hasCommandSubstitution detects $(...) and backtick `...` substitution.
// Allows $() inside single-quoted strings (where it's literal).
var cmdSubRegex = regexp.MustCompile(`\$\(|` + "`")

func hasCommandSubstitution(cmd string) bool {
	// Strip single-quoted segments (where $() is literal).
	stripped := stripSingleQuotes(cmd)
	return cmdSubRegex.MatchString(stripped)
}

// hasIFSInjection detects use of $IFS which enables word-splitting bypasses.
// e.g., cat$IFS/etc/passwd
var ifsRegex = regexp.MustCompile(`\$\{?IFS\}?`)

func hasIFSInjection(cmd string) bool {
	stripped := stripSingleQuotes(cmd)
	return ifsRegex.MatchString(stripped)
}

// hasDangerousBraceExpansion detects brace expansion patterns that could
// reassemble dangerous commands: {rm,-rf} expands to "rm -rf".
// Only flags comma-separated braces where a segment looks like a flag or
// known dangerous command.
var braceRegex = regexp.MustCompile(`\{[^}]*,[^}]*\}`)

// dangerousBraceWords are command names/flags that indicate hostile intent inside braces.
var dangerousBraceWords = []string{
	"rm", "mv", "cp", "chmod", "chown", "dd", "mkfs",
	"curl", "wget", "bash", "sh", "python", "perl", "ruby",
	"-rf", "-fr", "-f", "--force", "--hard",
}

func hasDangerousBraceExpansion(cmd string) bool {
	stripped := stripSingleQuotes(cmd)
	matches := braceRegex.FindAllString(stripped, -1)
	for _, m := range matches {
		inner := strings.ToLower(m[1 : len(m)-1]) // strip { }
		for _, word := range dangerousBraceWords {
			if strings.Contains(inner, word) {
				return true
			}
		}
	}
	return false
}

// hasProcessSubstitution detects <(...) and >(...) process substitution.
var procSubRegex = regexp.MustCompile(`[<>]\(`)

func hasProcessSubstitution(cmd string) bool {
	stripped := stripSingleQuotes(cmd)
	return procSubRegex.MatchString(stripped)
}

// hasBackslashOperators detects backslash-escaped shell operators that could
// cause parser differential attacks: \; \| \& in argument position.
// e.g., cat safe.txt \; rm -rf /
var backslashOpRegex = regexp.MustCompile(`\\[;|&]`)

func hasBackslashOperators(cmd string) bool {
	stripped := stripSingleQuotes(cmd)
	// Also strip double-quoted segments where backslash-escaping is expected.
	stripped = stripDoubleQuotes(stripped)
	return backslashOpRegex.MatchString(stripped)
}

// hasVariableRedirect detects variable expansion immediately before or after
// a redirect operator, which could target arbitrary files.
// e.g., echo x > $HOME/.bashrc, $VAR > /etc/config
var varRedirectRegex = regexp.MustCompile(
	`\$\{?\w+\}?\s*>` + // $VAR > or ${VAR} >
		`|>\s*\$\{?\w+\}?`, // > $VAR or > ${VAR}
)

func hasVariableRedirect(cmd string) bool {
	stripped := stripSingleQuotes(cmd)
	return varRedirectRegex.MatchString(stripped)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// stripSingleQuotes removes content inside single quotes where shell
// metacharacters are literal. Handles escaped single quotes (\').
func stripSingleQuotes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSingle := false
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\'' && !inSingle {
			inSingle = true
			continue
		}
		if r == '\'' && inSingle {
			inSingle = false
			continue
		}
		if !inSingle {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// stripDoubleQuotes removes content inside double quotes.
// Handles backslash escaping within double quotes.
func stripDoubleQuotes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inDouble := false
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\\' && inDouble && i+1 < len(runes) {
			i++ // skip escaped char
			continue
		}
		if r == '"' {
			inDouble = !inDouble
			continue
		}
		if !inDouble {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isAlphaOrUnderscore reports whether r is a valid shell variable character.
// Unused but retained for potential future use.
func isAlphaOrUnderscore(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
