package confirm

import (
	"context"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type Options struct {
	Root   string
	Files  []string
	NoTest bool
	NoLint bool
}

type Result struct {
	Pass     bool          `json:"pass"`
	Checks   []CheckResult `json:"checks"`
	Packages []string      `json:"packages,omitempty"`
	Files    []string      `json:"files,omitempty"`
	Elapsed  float64       `json:"elapsed_s"`
}

func Run(opts Options) (*Result, error) {
	root := opts.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	files := opts.Files
	if len(files) == 0 {
		var err error
		files, err = changedFiles(root)
		if err != nil {
			return nil, err
		}
	}

	if len(files) == 0 {
		return &Result{Pass: true}, nil
	}

	packages, err := AffectedPackages(files, root)
	if err != nil {
		return nil, err
	}

	if len(packages) == 0 {
		return &Result{Pass: true, Files: files}, nil
	}

	start := time.Now()
	var checks []CheckResult

	checks = append(checks, TypeCheck(packages, root))
	if !opts.NoTest {
		checks = append(checks, RunTests(packages, root))
	}
	if !opts.NoLint {
		checks = append(checks, Lint(files, root))
	}

	pass := true
	for _, c := range checks {
		if !c.Pass {
			pass = false
			break
		}
	}

	return &Result{
		Pass:     pass,
		Checks:   checks,
		Packages: packages,
		Files:    files,
		Elapsed:  time.Since(start).Seconds(),
	}, nil
}

func changedFiles(root string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collect := func(args ...string) ([]string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		var files []string
		for line := range strings.SplitSeq(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				files = append(files, line)
			}
		}
		return files, nil
	}

	staged, err := collect("diff", "--name-only", "HEAD")
	if err != nil {
		staged = nil
	}
	unstaged, err := collect("diff", "--name-only")
	if err != nil {
		unstaged = nil
	}

	seen := make(map[string]struct{})
	var merged []string
	for _, f := range append(staged, unstaged...) {
		if _, ok := seen[f]; !ok {
			seen[f] = struct{}{}
			merged = append(merged, f)
		}
	}
	sort.Strings(merged)
	return merged, nil
}
