// internal/consensus/consensus.go
package consensus

import (
	"regexp"
	"strings"
)

// Position represents a model's stance in the debate
type Position int

const (
	PositionUnknown Position = iota
	PositionAgree
	PositionObject
	PositionAdd
)

func (p Position) String() string {
	switch p {
	case PositionAgree:
		return "AGREE"
	case PositionObject:
		return "OBJECT"
	case PositionAdd:
		return "ADD"
	default:
		return "UNKNOWN"
	}
}

// ParsedPosition contains the detected position and extracted content
type ParsedPosition struct {
	Position   Position
	Target     string // For AGREE: which model they agree with
	Reason     string // For OBJECT: the objection reason
	Point      string // For ADD: the additional point
	RawContent string // The original content
}

// Patterns for parsing model responses
var (
	agreePattern  = regexp.MustCompile(`(?i)AGREE:\s*\[?([^\]\n]+)\]?`)
	objectPattern = regexp.MustCompile(`(?i)OBJECT:\s*(.+?)(?:\n|$)`)
	addPattern    = regexp.MustCompile(`(?i)ADD:\s*(.+?)(?:\n|$)`)

	// Fallback keyword patterns for implicit positions
	agreeKeywords  = []string{"i agree", "agreed", "concur", "support this", "that's correct", "exactly right"}
	objectKeywords = []string{"i disagree", "i object", "however", "but i think", "that's wrong", "incorrect"}
	addKeywords    = []string{"i would add", "additionally", "also consider", "one more thing", "to expand on"}
)

// DetectPosition analyzes content and returns the detected position type
func DetectPosition(content string) Position {
	parsed := ParseResponse(content)
	return parsed.Position
}

// ParseResponse parses a model response and extracts structured position data
func ParseResponse(content string) ParsedPosition {
	result := ParsedPosition{
		Position:   PositionUnknown,
		RawContent: content,
	}

	lower := strings.ToLower(content)

	// Check explicit patterns first (highest priority)
	if match := agreePattern.FindStringSubmatch(content); match != nil {
		result.Position = PositionAgree
		result.Target = strings.TrimSpace(match[1])
		return result
	}

	if match := objectPattern.FindStringSubmatch(content); match != nil {
		result.Position = PositionObject
		result.Reason = strings.TrimSpace(match[1])
		return result
	}

	if match := addPattern.FindStringSubmatch(content); match != nil {
		result.Position = PositionAdd
		result.Point = strings.TrimSpace(match[1])
		return result
	}

	// Fall back to keyword detection
	for _, kw := range agreeKeywords {
		if strings.Contains(lower, kw) {
			result.Position = PositionAgree
			return result
		}
	}

	for _, kw := range objectKeywords {
		if strings.Contains(lower, kw) {
			result.Position = PositionObject
			return result
		}
	}

	for _, kw := range addKeywords {
		if strings.Contains(lower, kw) {
			result.Position = PositionAdd
			return result
		}
	}

	return result
}

// ParseAgree extracts the target model from an AGREE statement
// Returns the model name and true if found, empty string and false otherwise
func ParseAgree(content string) (string, bool) {
	if match := agreePattern.FindStringSubmatch(content); match != nil {
		return strings.TrimSpace(match[1]), true
	}
	return "", false
}

// ParseObject extracts the reason from an OBJECT statement
// Returns the reason and true if found, empty string and false otherwise
func ParseObject(content string) (string, bool) {
	if match := objectPattern.FindStringSubmatch(content); match != nil {
		return strings.TrimSpace(match[1]), true
	}
	return "", false
}

// ParseAdd extracts the point from an ADD statement
// Returns the point and true if found, empty string and false otherwise
func ParseAdd(content string) (string, bool) {
	if match := addPattern.FindStringSubmatch(content); match != nil {
		return strings.TrimSpace(match[1]), true
	}
	return "", false
}

// CheckConsensus determines if there's consensus among the positions
// Returns true if a majority of participants agree
func CheckConsensus(positions map[string]Position) bool {
	if len(positions) == 0 {
		return false
	}

	agreeCount := 0
	objectCount := 0

	for _, pos := range positions {
		switch pos {
		case PositionAgree:
			agreeCount++
		case PositionObject:
			objectCount++
		// ADD positions are neutral - they don't block consensus
		}
	}

	// Consensus requires majority agreement and no objections
	// Or: majority agreement even with some objections (strict majority)
	majority := len(positions)/2 + 1
	return agreeCount >= majority && objectCount == 0
}

// CheckStrictConsensus requires all participants to agree (no objections, no unknowns)
func CheckStrictConsensus(positions map[string]Position) bool {
	if len(positions) == 0 {
		return false
	}

	for _, pos := range positions {
		if pos != PositionAgree && pos != PositionAdd {
			return false
		}
	}

	// At least one explicit agree
	for _, pos := range positions {
		if pos == PositionAgree {
			return true
		}
	}

	return false
}

// ConsensusResult contains detailed information about the consensus state
type ConsensusResult struct {
	HasConsensus    bool
	AgreeCount      int
	ObjectCount     int
	AddCount        int
	UnknownCount    int
	TotalCount      int
	AgreementTarget string   // Most agreed-upon model, if any
	Objections      []string // List of objection reasons
	Additions       []string // List of additional points
}

// AnalyzeConsensus performs detailed consensus analysis on parsed positions
func AnalyzeConsensus(positions map[string]ParsedPosition) ConsensusResult {
	result := ConsensusResult{
		TotalCount: len(positions),
	}

	if len(positions) == 0 {
		return result
	}

	targetCounts := make(map[string]int)

	for _, parsed := range positions {
		switch parsed.Position {
		case PositionAgree:
			result.AgreeCount++
			if parsed.Target != "" {
				targetCounts[parsed.Target]++
			}
		case PositionObject:
			result.ObjectCount++
			if parsed.Reason != "" {
				result.Objections = append(result.Objections, parsed.Reason)
			}
		case PositionAdd:
			result.AddCount++
			if parsed.Point != "" {
				result.Additions = append(result.Additions, parsed.Point)
			}
		default:
			result.UnknownCount++
		}
	}

	// Find most agreed-upon target
	maxCount := 0
	for target, count := range targetCounts {
		if count > maxCount {
			maxCount = count
			result.AgreementTarget = target
		}
	}

	// Determine consensus
	majority := len(positions)/2 + 1
	result.HasConsensus = result.AgreeCount >= majority && result.ObjectCount == 0

	return result
}
