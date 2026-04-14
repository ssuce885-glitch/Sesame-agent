package intent

import "strings"

type CapabilityProfile string

const (
	ProfileCodebaseEdit      CapabilityProfile = "codebase_edit"
	ProfileSystemInspect     CapabilityProfile = "system_inspect"
	ProfileWebLookup         CapabilityProfile = "web_lookup"
	ProfileBrowserAutomation CapabilityProfile = "browser_automation"
	ProfileAutomation        CapabilityProfile = "automation"
	ProfileScheduledReport   CapabilityProfile = "scheduled_report"
)

const defaultConfirmationText = "I can either build long-running automation or schedule a one-time/recurring report. Which one should I do?"

type Plan struct {
	Signal       Signal
	Profile      CapabilityProfile
	NeedsConfirm bool
	ConfirmText  string
	Fallback     CapabilityProfile
}

// Resolve is kept as a narrow compatibility wrapper while callers migrate.
func Resolve(signal Signal) Plan {
	return planFromClassifierResult(signal, FallbackClassify(signal.Raw))
}

func planFromClassifierResult(signal Signal, result ClassifierResult) Plan {
	result = normalizeClassifierResult(result)
	return Plan{
		Signal:       signal,
		Profile:      result.Profile,
		NeedsConfirm: result.NeedsConfirm,
		ConfirmText:  strings.TrimSpace(result.ConfirmText),
		Fallback:     result.FallbackProfile,
	}
}

func normalizeClassifierResult(result ClassifierResult) ClassifierResult {
	result.Profile = normalizeProfile(result.Profile)
	result.FallbackProfile = normalizeFallbackProfile(result.FallbackProfile)
	result.ConfirmText = strings.TrimSpace(result.ConfirmText)
	if result.NeedsConfirm {
		if result.Profile == ProfileAutomation && result.FallbackProfile == ProfileCodebaseEdit {
			result.FallbackProfile = ProfileScheduledReport
		}
		if result.ConfirmText == "" {
			result.ConfirmText = defaultConfirmationText
		}
	}
	return result
}

func normalizeProfile(profile CapabilityProfile) CapabilityProfile {
	switch profile {
	case ProfileAutomation, ProfileScheduledReport, ProfileBrowserAutomation, ProfileWebLookup, ProfileSystemInspect, ProfileCodebaseEdit:
		return profile
	default:
		return ProfileCodebaseEdit
	}
}

func normalizeFallbackProfile(profile CapabilityProfile) CapabilityProfile {
	switch profile {
	case ProfileAutomation, ProfileScheduledReport, ProfileBrowserAutomation, ProfileWebLookup, ProfileSystemInspect, ProfileCodebaseEdit:
		return profile
	default:
		return ProfileCodebaseEdit
	}
}
