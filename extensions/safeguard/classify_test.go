package safeguard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyCommand_ReadOnly(t *testing.T) {
	t.Parallel()
	cases := []string{
		"ls -la",
		"cat file.txt",
		"head -20 main.go",
		"tail -f /var/log/syslog",
		"grep -rn pattern .",
		"rg 'func main' --type go",
		"wc -l *.go",
		"diff a.txt b.txt",
		"echo hello world",
		"pwd",
		"env",
		"whoami",
		"date +%Y-%m-%d",
		"which go",
		"ps aux",
		"du -sh .",
		"stat file.txt",
		"basename /path/to/file.txt",
		"id",
		"sort < input.txt",
		"jq '.name' package.json",
		"tree -L 2",
		"true",
		"test -f file.txt",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, CommandReadOnly, ClassifyCommand(cmd), "expected read_only")
		})
	}
}

func TestClassifyCommand_Write(t *testing.T) {
	t.Parallel()
	cases := []string{
		"rm file.txt",
		"rm -rf /tmp/junk",
		"mv old.txt new.txt",
		"cp src.txt dst.txt",
		"chmod 755 script.sh",
		"mkdir -p foo/bar",
		"touch newfile.txt",
		"ln -s target link",
		"kill -9 1234",
		"echo hello > output.txt",
		"cat file >> log.txt",
		"ls > listing.txt",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, CommandWrite, ClassifyCommand(cmd), "expected write")
		})
	}
}

func TestClassifyCommand_Unknown(t *testing.T) {
	t.Parallel()
	cases := []string{
		"curl https://example.com",
		"wget https://example.com",
		"python3 script.py",
		"make",
		"go build ./...",
		"go test -race ./...",
		"npm install",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, CommandUnknown, ClassifyCommand(cmd), "expected unknown")
		})
	}
}

func TestClassifyCommand_Pipes(t *testing.T) {
	t.Parallel()

	t.Run("read pipe read", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("cat file.txt | grep pattern"))
	})

	t.Run("read pipe read pipe read", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("ps aux | grep go | wc -l"))
	})

	t.Run("read pipe write", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandWrite, ClassifyCommand("echo hello | tee output.txt"))
	})

	t.Run("read pipe unknown", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandUnknown, ClassifyCommand("ls | python3 -c 'import sys'"))
	})
}

func TestClassifyCommand_Chains(t *testing.T) {
	t.Parallel()

	t.Run("read && read", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("cd /tmp && ls -la"))
	})

	t.Run("read ; read", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("pwd; ls"))
	})

	t.Run("read && write", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandWrite, ClassifyCommand("cd /tmp && rm -rf junk"))
	})

	t.Run("read || read", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("cat file.txt || echo 'not found'"))
	})
}

func TestClassifyCommand_Redirects(t *testing.T) {
	t.Parallel()

	t.Run("stderr redirect is not write", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("ls 2>/dev/null"))
	})

	t.Run("stderr dup is not write", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("echo hello 2>&1"))
	})

	t.Run("stdout redirect is write", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandWrite, ClassifyCommand("echo hello > file.txt"))
	})

	t.Run("append redirect is write", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandWrite, ClassifyCommand("echo hello >> file.txt"))
	})

	t.Run("fd redirect is not write", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("echo hello >&2"))
	})
}

func TestClassifyCommand_Git(t *testing.T) {
	t.Parallel()

	readCases := []string{
		"git status",
		"git log --oneline -10",
		"git diff HEAD~1",
		"git show abc123",
		"git blame file.go",
		"git ls-files",
		"git rev-parse HEAD",
		"git remote -v",
		"git describe --tags",
	}
	for _, cmd := range readCases {
		t.Run(cmd+" read", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, CommandReadOnly, ClassifyCommand(cmd))
		})
	}

	writeCases := []string{
		"git push origin main",
		"git pull",
		"git commit -m 'hello'",
		"git merge feature",
		"git rebase main",
		"git reset --hard HEAD~1",
		"git clean -fd",
		"git checkout feature",
		"git stash",
		"git add .",
		"git branch -D old",
		"git tag v1.0",
	}
	for _, cmd := range writeCases {
		t.Run(cmd+" write", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, CommandWrite, ClassifyCommand(cmd))
		})
	}
}

func TestClassifyCommand_Find(t *testing.T) {
	t.Parallel()

	t.Run("read-only find", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("find . -name '*.go' -type f"))
	})

	t.Run("find with -exec is write", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandWrite, ClassifyCommand("find . -name '*.tmp' -exec rm {} \\;"))
	})

	t.Run("find with -delete is write", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandWrite, ClassifyCommand("find /tmp -name '*.log' -delete"))
	})
}

func TestClassifyCommand_Sed(t *testing.T) {
	t.Parallel()

	t.Run("sed without -i is read", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("sed 's/foo/bar/g' file.txt"))
	})

	t.Run("sed -i is write", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandWrite, ClassifyCommand("sed -i 's/foo/bar/g' file.txt"))
	})

	t.Run("sed --in-place is write", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandWrite, ClassifyCommand("sed --in-place 's/foo/bar/' file.txt"))
	})

	t.Run("sed -inE combined flag is write", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandWrite, ClassifyCommand("sed -inE 's/old/new/' file.txt"))
	})
}

func TestClassifyCommand_Go(t *testing.T) {
	t.Parallel()

	t.Run("go version", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("go version"))
	})

	t.Run("go env", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("go env GOPATH"))
	})

	t.Run("go list", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("go list ./..."))
	})

	t.Run("go build is unknown", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandUnknown, ClassifyCommand("go build ./..."))
	})

	t.Run("go test is unknown", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandUnknown, ClassifyCommand("go test -race ./..."))
	})
}

func TestClassifyCommand_Docker(t *testing.T) {
	t.Parallel()

	t.Run("docker ps", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("docker ps"))
	})

	t.Run("docker images", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("docker images"))
	})

	t.Run("docker run is unknown", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandUnknown, ClassifyCommand("docker run alpine ls"))
	})
}

func TestClassifyCommand_EnvVarPrefix(t *testing.T) {
	t.Parallel()

	t.Run("env var before read cmd", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("FOO=bar ls -la"))
	})

	t.Run("env var before write cmd", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandWrite, ClassifyCommand("FOO=bar rm file.txt"))
	})
}

func TestClassifyCommand_CommandSubstitution(t *testing.T) {
	t.Parallel()

	t.Run("dollar-paren is unknown", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandUnknown, ClassifyCommand("echo $(whoami)"))
	})

	t.Run("backtick is unknown", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandUnknown, ClassifyCommand("echo `date`"))
	})

	t.Run("single-quoted dollar-paren is safe", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, CommandReadOnly, ClassifyCommand("echo '$(whoami)'"))
	})
}

func TestClassifyCommand_Empty(t *testing.T) {
	t.Parallel()
	assert.Equal(t, CommandUnknown, ClassifyCommand(""))
	assert.Equal(t, CommandUnknown, ClassifyCommand("   "))
}

func TestCommandKind_String(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "read_only", CommandReadOnly.String())
	assert.Equal(t, "write", CommandWrite.String())
	assert.Equal(t, "unknown", CommandUnknown.String())
}

func TestSplitSegments(t *testing.T) {
	t.Parallel()

	t.Run("simple", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"ls"}, splitSegments("ls"))
	})

	t.Run("semicolon", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"ls", "pwd"}, splitSegments("ls; pwd"))
	})

	t.Run("and", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"cd /tmp", "ls"}, splitSegments("cd /tmp && ls"))
	})

	t.Run("pipe", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"cat f", "grep x"}, splitSegments("cat f | grep x"))
	})

	t.Run("quoted semicolon preserved", func(t *testing.T) {
		t.Parallel()
		segs := splitSegments(`echo "hello; world"`)
		assert.Len(t, segs, 1)
	})
}

func TestExtractBaseCommand(t *testing.T) {
	t.Parallel()

	t.Run("simple", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "ls", extractBaseCommand("ls -la"))
	})

	t.Run("env prefix", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "cmd", extractBaseCommand("FOO=bar cmd arg"))
	})

	t.Run("subshell paren", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "ls", extractBaseCommand("(ls -la)"))
	})
}

func TestHasOutputRedirect(t *testing.T) {
	t.Parallel()

	t.Run("stdout redirect", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasOutputRedirect("> file"))
	})

	t.Run("stderr redirect skipped", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasOutputRedirect("2>/dev/null"))
	})

	t.Run("fd dup skipped", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasOutputRedirect(">&2"))
	})

	t.Run("append redirect", func(t *testing.T) {
		t.Parallel()
		assert.True(t, hasOutputRedirect(">> file"))
	})
}
