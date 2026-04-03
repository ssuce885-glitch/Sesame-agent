package memory

func ExtractCandidates(userMessage, assistantMessage string) []Candidate {
	out := make([]Candidate, 0, 2)
	if userMessage != "" {
		out = append(out, Candidate{Content: userMessage, Confidence: 0.5})
	}
	if assistantMessage != "" {
		out = append(out, Candidate{Content: assistantMessage, Confidence: 0.7})
	}

	return out
}
