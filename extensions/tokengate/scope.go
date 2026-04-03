package tokengate

import (
	"context"
	"maps"
	"regexp"
	"strconv"
	"strings"
)

type compiledRule struct {
	tool    string
	pattern *regexp.Regexp
	action  string
	value   string
}

func scopeBeforeInterceptor(cfg Config) func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
	compiled := make([]compiledRule, 0, len(cfg.Rules))
	for _, r := range cfg.Rules {
		if r.Pattern == "" {
			compiled = append(compiled, compiledRule{tool: r.Tool, action: r.Action, value: r.Value})
			continue
		}
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			continue
		}
		compiled = append(compiled, compiledRule{tool: r.Tool, pattern: re, action: r.Action, value: r.Value})
	}

	return func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
		for _, rule := range compiled {
			if rule.tool != toolName {
				continue
			}
			switch toolName {
			case "bash":
				return rewriteBash(args, rule)
			case "Read":
				return rewriteRead(args, rule)
			case "Grep":
				return rewriteGrep(args, rule)
			}
		}
		return true, args, nil
	}
}

func rewriteBash(args map[string]any, rule compiledRule) (bool, map[string]any, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return true, args, nil
	}
	if rule.pattern != nil && !rule.pattern.MatchString(command) {
		return true, args, nil
	}
	if rule.action != "append_head" {
		return true, args, nil
	}
	if alreadyScoped(command) {
		return true, args, nil
	}
	modified := maps.Clone(args)
	modified["command"] = command + " | head -" + rule.value
	return true, modified, nil
}

func rewriteRead(args map[string]any, rule compiledRule) (bool, map[string]any, error) {
	if rule.action != "limit_lines" {
		return true, args, nil
	}
	if _, hasLimit := args["limit"]; hasLimit {
		return true, args, nil
	}
	if _, hasOffset := args["offset"]; hasOffset {
		return true, args, nil
	}
	n, err := strconv.Atoi(rule.value)
	if err != nil {
		return true, args, nil
	}
	modified := maps.Clone(args)
	modified["limit"] = float64(n)
	return true, modified, nil
}

func rewriteGrep(args map[string]any, rule compiledRule) (bool, map[string]any, error) {
	if rule.action != "limit_lines" {
		return true, args, nil
	}
	if _, hasHeadLimit := args["head_limit"]; hasHeadLimit {
		return true, args, nil
	}
	n, err := strconv.Atoi(rule.value)
	if err != nil {
		return true, args, nil
	}
	modified := maps.Clone(args)
	modified["head_limit"] = float64(n)
	return true, modified, nil
}

func alreadyScoped(command string) bool {
	return strings.Contains(command, "| head") || strings.Contains(command, "| tail")
}
