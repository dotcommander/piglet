package pipeline

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Iteration represents one execution of a looped step.
type Iteration struct {
	Item     string            // value from each list
	LoopVars map[string]string // values from loop ranges
}

// ExpandIterations computes all iterations for a step.
// If neither each nor loop is set, returns nil (single execution).
// If both are set, returns cartesian product.
func ExpandIterations(step *Step) ([]Iteration, error) {
	hasEach := len(step.Each) > 0
	hasLoop := len(step.Loop) > 0

	if !hasEach && !hasLoop {
		return nil, nil
	}

	// Expand loop ranges into concrete values
	var loopSets []loopDimension
	if hasLoop {
		for key, raw := range step.Loop {
			values, err := expandRange(raw)
			if err != nil {
				return nil, fmt.Errorf("loop %q: %w", key, err)
			}
			loopSets = append(loopSets, loopDimension{key: key, values: values})
		}
	}

	// Compute loop cartesian product
	loopCombinations := cartesianLoop(loopSets)

	// Build iterations
	if hasEach && hasLoop {
		// Cartesian product of each × loop
		iters := make([]Iteration, 0, len(step.Each)*len(loopCombinations))
		for _, item := range step.Each {
			for _, lc := range loopCombinations {
				iters = append(iters, Iteration{Item: item, LoopVars: lc})
			}
		}
		return iters, nil
	}

	if hasEach {
		iters := make([]Iteration, len(step.Each))
		for i, item := range step.Each {
			iters[i] = Iteration{Item: item}
		}
		return iters, nil
	}

	// hasLoop only
	iters := make([]Iteration, len(loopCombinations))
	for i, lc := range loopCombinations {
		iters[i] = Iteration{LoopVars: lc}
	}
	return iters, nil
}

type loopDimension struct {
	key    string
	values []string
}

// expandRange converts a loop value to a list of strings.
// Supports: explicit list ([]any), numeric range ("1..5"), time range ("-7d..-1d"), single string.
func expandRange(raw any) ([]string, error) {
	switch v := raw.(type) {
	case []any:
		result := make([]string, len(v))
		for i, item := range v {
			result[i] = fmt.Sprint(item)
		}
		return result, nil
	case string:
		return parseRange(v)
	default:
		return []string{fmt.Sprint(v)}, nil
	}
}

// parseRange parses "start..end" range expressions.
// Numeric: "1..5" → ["1","2","3","4","5"]
// Time relative days: "-7d..-1d" → ["-7d","-6d",...,"-1d"]
func parseRange(s string) ([]string, error) {
	parts := strings.SplitN(s, "..", 2)
	if len(parts) != 2 {
		// Not a range — treat as single value
		return []string{s}, nil
	}

	startStr, endStr := parts[0], parts[1]

	// Time range (days): "-7d..-1d"
	if strings.HasSuffix(startStr, "d") && strings.HasSuffix(endStr, "d") {
		startDays, err := strconv.Atoi(strings.TrimSuffix(startStr, "d"))
		if err != nil {
			return nil, fmt.Errorf("invalid day range start %q: %w", startStr, err)
		}
		endDays, err := strconv.Atoi(strings.TrimSuffix(endStr, "d"))
		if err != nil {
			return nil, fmt.Errorf("invalid day range end %q: %w", endStr, err)
		}
		return expandDayRange(startDays, endDays, time.Time{}), nil
	}

	// Numeric range
	start, err := strconv.Atoi(startStr)
	if err != nil {
		// Not numeric — treat as single value
		return []string{s}, nil
	}
	end, err := strconv.Atoi(endStr)
	if err != nil {
		return []string{s}, nil
	}

	return expandNumericRange(start, end), nil
}

func expandNumericRange(start, end int) []string {
	if start > end {
		result := make([]string, 0, start-end+1)
		for i := start; i >= end; i-- {
			result = append(result, strconv.Itoa(i))
		}
		return result
	}
	result := make([]string, 0, end-start+1)
	for i := start; i <= end; i++ {
		result = append(result, strconv.Itoa(i))
	}
	return result
}

func expandDayRange(startDays, endDays int, ref time.Time) []string {
	if ref.IsZero() {
		ref = time.Now()
	}
	if startDays > endDays {
		startDays, endDays = endDays, startDays
	}
	result := make([]string, 0, endDays-startDays+1)
	for d := startDays; d <= endDays; d++ {
		date := ref.AddDate(0, 0, d)
		result = append(result, date.Format("2006-01-02"))
	}
	return result
}

// cartesianLoop computes the cartesian product of all loop dimensions.
// Returns a slice of maps, each map being one combination.
func cartesianLoop(dims []loopDimension) []map[string]string {
	if len(dims) == 0 {
		return []map[string]string{{}}
	}

	first := dims[0]
	rest := cartesianLoop(dims[1:])

	result := make([]map[string]string, 0, len(first.values)*len(rest))
	for _, val := range first.values {
		for _, r := range rest {
			combo := make(map[string]string, len(r)+1)
			for k, v := range r {
				combo[k] = v
			}
			combo[first.key] = val
			result = append(result, combo)
		}
	}
	return result
}
