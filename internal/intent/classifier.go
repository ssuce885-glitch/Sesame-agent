package intent

import "context"

type Classifier interface {
	Classify(ctx context.Context, message string) (ClassifierResult, error)
}

type ClassifierResult struct {
	Profile         CapabilityProfile `json:"profile"`
	FallbackProfile CapabilityProfile `json:"fallback_profile,omitempty"`
	NeedsConfirm    bool              `json:"needs_confirm"`
	ConfirmText     string            `json:"confirm_text,omitempty"`
	Modifiers       []string          `json:"modifiers,omitempty"`
}

func ClassifyIntent(ctx context.Context, classifier Classifier, message string, catalog SkillCatalog) (Plan, error) {
	signal := ExtractSkillSignals(message, catalog)
	if classifier == nil {
		return planFromClassifierResult(signal, FallbackClassify(message)), nil
	}
	result, err := classifier.Classify(ctx, message)
	if err != nil {
		result = FallbackClassify(message)
	}
	return planFromClassifierResult(signal, result), nil
}
