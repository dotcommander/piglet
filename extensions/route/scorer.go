package route

import (
	"cmp"
	"slices"
	"strings"
)

// RouteResult is the output of the scoring pipeline.
type RouteResult struct {
	Primary    []ScoredComponent `json:"primary"`
	Secondary  []ScoredComponent `json:"secondary,omitempty"`
	Intent     IntentResult      `json:"intent"`
	Domains    []string          `json:"domains"`
	Confidence float64           `json:"confidence"`
}

// ScoredComponent is a component with its composite score.
type ScoredComponent struct {
	Name      string        `json:"name"`
	Type      ComponentType `json:"type"`
	Extension string        `json:"extension,omitempty"`
	Score     float64       `json:"score"`
	Matched   []string      `json:"matched,omitempty"` // which triggers/keywords matched
}

// Scorer runs the weighted scoring pipeline.
type Scorer struct {
	config  Config
	intent  *IntentClassifier
	domains *DomainExtractor
}

// NewScorer creates a scorer with the given configuration.
func NewScorer(cfg Config, ic *IntentClassifier, de *DomainExtractor) *Scorer {
	return &Scorer{config: cfg, intent: ic, domains: de}
}

// Score runs the full pipeline: tokenize → classify intent → extract domains → score → tier.
func (s *Scorer) Score(prompt, projectDir string, reg *Registry) RouteResult {
	tokens := Tokenize(prompt)
	intent := s.intent.Classify(prompt)
	domains := s.domains.Extract(prompt, projectDir)

	var scored []ScoredComponent
	for _, comp := range reg.Components {
		sc := s.scoreComponent(tokens, intent, domains, comp)
		if sc.Score > 0 {
			scored = append(scored, sc)
		}
	}

	// Sort by score descending
	slices.SortFunc(scored, func(a, b ScoredComponent) int {
		return cmp.Compare(b.Score, a.Score)
	})

	// Split into tiers
	result := RouteResult{
		Intent:  intent,
		Domains: domains,
	}

	for _, sc := range scored {
		if sc.Score >= s.config.PrimaryThreshold {
			if len(result.Primary) < s.config.MaxPrimary {
				result.Primary = append(result.Primary, sc)
			}
		} else if len(result.Secondary) < s.config.MaxSecondary {
			result.Secondary = append(result.Secondary, sc)
		}
	}

	// Confidence = gap between top two primary scores
	if len(result.Primary) >= 2 {
		result.Confidence = result.Primary[0].Score - result.Primary[1].Score
	} else if len(result.Primary) == 1 {
		result.Confidence = result.Primary[0].Score
	}

	return result
}

func (s *Scorer) scoreComponent(tokens []string, intent IntentResult, domains []string, comp Component) ScoredComponent {
	w := s.config.Weights

	intentScore := s.scoreIntent(intent, comp)
	domainScore := s.scoreDomain(domains, comp)
	triggerScore, matched := s.scoreTriggers(tokens, comp)

	score := intentScore*w.Intent + domainScore*w.Domain + triggerScore*w.Trigger

	// Apply anti-trigger penalty
	if antiScore := s.scoreAntiTriggers(tokens, comp); antiScore > 0 {
		score *= 1.0 - antiScore*w.Anti
	}

	return ScoredComponent{
		Name:      comp.Name,
		Type:      comp.Type,
		Extension: comp.Extension,
		Score:     score,
		Matched:   matched,
	}
}

// scoreIntent checks if the classified intent matches the component.
// Prefers declared intents from manifest; falls back to name/description heuristics.
func (s *Scorer) scoreIntent(intent IntentResult, comp Component) float64 {
	if intent.Primary == "" {
		return 0
	}

	// Declared intents from manifest — strongest signal
	if len(comp.Intents) > 0 {
		for _, ci := range comp.Intents {
			if strings.EqualFold(ci, intent.Primary) {
				return 1.0
			}
			if intent.Secondary != "" && strings.EqualFold(ci, intent.Secondary) {
				return 0.7
			}
		}
		return 0
	}

	// Fallback: heuristic name/description matching for un-enriched components
	lower := strings.ToLower(comp.Name)
	desc := strings.ToLower(comp.Description)

	if strings.Contains(lower, intent.Primary) || strings.Contains(desc, intent.Primary) {
		return 0.6
	}

	return 0
}

// scoreDomain checks overlap between detected domains and component.
// Prefers declared domains from manifest; falls back to keyword heuristics.
func (s *Scorer) scoreDomain(domains []string, comp Component) float64 {
	if len(domains) == 0 {
		return 0
	}

	matches := 0

	// Declared domains from manifest — strongest signal
	if len(comp.Domains) > 0 {
		for _, d := range domains {
			for _, cd := range comp.Domains {
				if strings.EqualFold(d, cd) {
					matches++
					break
				}
			}
		}
		if matches == 0 {
			return 0
		}
		return float64(matches) / float64(len(domains))
	}

	// Fallback: check if domain appears in component name or keywords
	for _, d := range domains {
		lower := strings.ToLower(d)
		if strings.Contains(strings.ToLower(comp.Name), lower) {
			matches++
			continue
		}
		for _, kw := range comp.Keywords {
			if kw == lower || strings.Contains(kw, lower) {
				matches++
				break
			}
		}
	}

	if matches == 0 {
		return 0
	}
	return float64(matches) / float64(len(domains))
}

// scoreAntiTriggers returns 0..1 representing how strongly anti-triggers match.
// Higher values mean stronger penalty should be applied.
func (s *Scorer) scoreAntiTriggers(tokens []string, comp Component) float64 {
	if len(tokens) == 0 || len(comp.AntiTriggers) == 0 {
		return 0
	}

	matches := 0
	for _, at := range comp.AntiTriggers {
		if TokensContain(tokens, at) {
			matches++
		}
	}

	if matches == 0 {
		return 0
	}
	return float64(matches) / float64(len(comp.AntiTriggers))
}

// scoreTriggers scores keyword/trigger overlap between prompt tokens and component.
func (s *Scorer) scoreTriggers(tokens []string, comp Component) (float64, []string) {
	if len(tokens) == 0 {
		return 0, nil
	}

	var matched []string
	ratio := s.config.TriggerKeywordRatio

	// Score multi-word triggers (weighted higher)
	triggerMatches := 0
	for _, trig := range comp.Triggers {
		if TokensContainAll(tokens, trig) {
			triggerMatches++
			matched = append(matched, trig)
		}
	}

	// Score single-word keyword matches
	kwMatches := 0
	for _, kw := range comp.Keywords {
		if TokensContain(tokens, kw) {
			kwMatches++
			matched = append(matched, kw)
		}
	}

	if len(comp.Triggers)+len(comp.Keywords) == 0 {
		return 0, nil
	}

	// Weighted combination
	triggerTotal := max(len(comp.Triggers), 1)
	kwTotal := max(len(comp.Keywords), 1)

	triggerRatio := float64(triggerMatches) / float64(triggerTotal)
	kwRatio := float64(kwMatches) / float64(kwTotal)

	score := triggerRatio*ratio + kwRatio*(1-ratio)
	return score, matched
}
