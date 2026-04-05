package cli

// LevenshteinDistance returns the edit distance between strings a and b.
func LevenshteinDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)

	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// dp[j] = distance between a[:i] and b[:j]
	dp := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		dp[j] = j
	}

	for i := 1; i <= la; i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= lb; j++ {
			tmp := dp[j]
			if ra[i-1] == rb[j-1] {
				dp[j] = prev
			} else {
				dp[j] = 1 + min3(prev, dp[j], dp[j-1])
			}
			prev = tmp
		}
	}
	return dp[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// confidenceForDistance maps Levenshtein distance to confidence score.
// Distance 0 → 1.0, 1 → 0.9, 2 → 0.8, >2 → 0.
func confidenceForDistance(d int) float64 {
	switch d {
	case 0:
		return 1.0
	case 1:
		return 0.9
	case 2:
		return 0.8
	default:
		return 0
	}
}

// FuzzyMatchVerb finds the best verb match within Levenshtein distance ≤2.
// Returns (VerbCategory, confidence) or ("", 0) if no match.
func FuzzyMatchVerb(word string) (VerbCategory, float64) {
	if word == "" {
		return "", 0
	}
	// Try exact first
	if vc, ok := LookupVerb(word); ok {
		return vc, 1.0
	}

	var bestClass VerbCategory
	var bestKey string
	bestDist := 3 // >2 means no match

	for candidate, vc := range verbAliases {
		d := LevenshteinDistance(word, candidate)
		if d < bestDist || (d == bestDist && candidate < bestKey) {
			bestDist = d
			bestKey = candidate
			bestClass = vc
		}
	}

	conf := confidenceForDistance(bestDist)
	if conf == 0 {
		return "", 0
	}
	return bestClass, conf
}

// FuzzyMatchNoun finds the best noun match within Levenshtein distance ≤2.
// Returns (NounDomain, confidence) or ("", 0) if no match.
func FuzzyMatchNoun(word string) (NounDomain, float64) {
	if word == "" {
		return "", 0
	}
	// Try exact first
	if nd, ok := LookupNoun(word); ok {
		return nd, 1.0
	}

	var bestDomain NounDomain
	var bestKey string
	bestDist := 3 // >2 means no match

	for candidate, nd := range nounAliases {
		d := LevenshteinDistance(word, candidate)
		if d < bestDist || (d == bestDist && candidate < bestKey) {
			bestDist = d
			bestKey = candidate
			bestDomain = nd
		}
	}

	conf := confidenceForDistance(bestDist)
	if conf == 0 {
		return "", 0
	}
	return bestDomain, conf
}
