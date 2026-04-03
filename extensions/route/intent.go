package route

import (
	"strings"
)

// IntentResult holds the classified intent for a prompt.
type IntentResult struct {
	Primary    string  // primary intent name
	Confidence float64 // 0.0-1.0
	Secondary  string  // optional secondary intent
}

// IntentClassifier classifies prompts into intents using rule-based cascade.
type IntentClassifier struct {
	intents map[string]IntentDef
}

// NewIntentClassifier creates a classifier from the intent taxonomy.
func NewIntentClassifier(cfg IntentsConfig) *IntentClassifier {
	return &IntentClassifier{intents: cfg.Intents}
}

// Classify runs a four-rule cascade to determine prompt intent.
//
// Priority:
//  1. Question detection (finds action verb after filler words)
//  2. Leading verb match
//  3. Problem keywords (no verb found)
//  4. Best keyword overlap
func (ic *IntentClassifier) Classify(text string) IntentResult {
	lower := strings.ToLower(strings.TrimSpace(text))
	tokens := Tokenize(text)

	if len(tokens) == 0 {
		return IntentResult{}
	}

	// Rule 1: Question detection — strip question words, find action verb
	if isQuestion(lower) {
		verb, intent := ic.findVerbInTokens(tokens)
		if verb != "" {
			secondary := ic.findSecondaryIntent(tokens, intent)
			return IntentResult{Primary: intent, Confidence: 0.85, Secondary: secondary}
		}
	}

	// Rule 2: Leading verb match — first token matches an intent verb
	for name, def := range ic.intents {
		for _, verb := range def.Verbs {
			if tokens[0] == verb || NormalizePlural(tokens[0]) == verb {
				secondary := ic.findSecondaryIntent(tokens, name)
				return IntentResult{Primary: name, Confidence: 0.9, Secondary: secondary}
			}
		}
	}

	// Rule 3: Problem keywords without a leading verb
	for name, def := range ic.intents {
		for _, kw := range def.Keywords {
			if strings.Contains(lower, kw) {
				secondary := ic.findSecondaryIntent(tokens, name)
				return IntentResult{Primary: name, Confidence: 0.7, Secondary: secondary}
			}
		}
	}

	// Rule 4: Best keyword overlap
	bestName := ""
	bestCount := 0
	for name, def := range ic.intents {
		count := 0
		for _, kw := range def.Keywords {
			if strings.Contains(lower, kw) {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			bestName = name
		}
	}
	if bestName != "" {
		return IntentResult{Primary: bestName, Confidence: 0.6}
	}

	return IntentResult{}
}

// findVerbInTokens scans all tokens for a verb match, skipping question/filler words.
func (ic *IntentClassifier) findVerbInTokens(tokens []string) (string, string) {
	for _, t := range tokens {
		for name, def := range ic.intents {
			for _, verb := range def.Verbs {
				if t == verb || NormalizePlural(t) == verb {
					return verb, name
				}
			}
		}
	}
	return "", ""
}

// findSecondaryIntent scans remaining tokens for verb matches to detect
// compound intents like "refactor and test".
func (ic *IntentClassifier) findSecondaryIntent(tokens []string, primary string) string {
	for _, t := range tokens {
		for name, def := range ic.intents {
			if name == primary {
				continue
			}
			for _, verb := range def.Verbs {
				if t == verb || NormalizePlural(t) == verb {
					return name
				}
			}
		}
	}
	return ""
}

// questionPrefixes are words that start questions.
var questionPrefixes = []string{
	"how do", "how can", "how to", "how should",
	"what is", "what are", "what does", "what's",
	"where is", "where are", "where does",
	"why is", "why are", "why does", "why did",
	"when does", "when should", "when is",
	"can you", "could you", "would you", "will you",
	"is there", "are there", "do we", "does it",
}

func isQuestion(lower string) bool {
	if strings.HasSuffix(strings.TrimSpace(lower), "?") {
		return true
	}
	for _, prefix := range questionPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}
