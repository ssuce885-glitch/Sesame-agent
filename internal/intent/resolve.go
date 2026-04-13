package intent

import (
	"fmt"
	"sort"
)

type CapabilityProfile string

const (
	ProfileCodebaseEdit      CapabilityProfile = "codebase_edit"
	ProfileSystemInspect     CapabilityProfile = "system_inspect"
	ProfileWebLookup         CapabilityProfile = "web_lookup"
	ProfileBrowserAutomation CapabilityProfile = "browser_automation"
	ProfileAutomation        CapabilityProfile = "automation"
	ProfileScheduledReport   CapabilityProfile = "scheduled_report"
)

type Plan struct {
	Signal       Signal
	Primary      Flag
	Secondary    []Flag
	Profile      CapabilityProfile
	NeedsConfirm bool
	ConfirmText  string
	Fallback     CapabilityProfile
}

func Resolve(signal Signal) Plan {
	ordered := orderedFlags(signal)
	primary := FlagCodeEdit
	if len(ordered) > 0 {
		primary = ordered[0]
	}

	plan := Plan{
		Signal:    signal,
		Primary:   primary,
		Secondary: append([]Flag(nil), ordered[1:]...),
		Profile:   profileForFlag(primary),
		Fallback:  ProfileCodebaseEdit,
	}

	if unresolvedAutomationScheduling(signal) {
		plan.NeedsConfirm = true
		plan.Profile = ProfileAutomation
		plan.ConfirmText = "I can either build long-running automation or schedule a one-time/recurring report. Which one should I do?"
	}

	return plan
}

func orderedFlags(signal Signal) []Flag {
	if len(signal.Flags) == 0 {
		return []Flag{FlagCodeEdit}
	}
	out := make([]Flag, 0, len(signal.Flags))
	for flag, enabled := range signal.Flags {
		if enabled {
			out = append(out, flag)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left := signal.Strength[out[i]]
		right := signal.Strength[out[j]]
		if left == right {
			return out[i] < out[j]
		}
		return left > right
	})
	return out
}

func unresolvedAutomationScheduling(signal Signal) bool {
	if !signal.Flags[FlagAutomation] || !signal.Flags[FlagScheduling] {
		return false
	}
	autoStrength := strengthOrDefault(signal, FlagAutomation)
	schedStrength := strengthOrDefault(signal, FlagScheduling)
	diff := autoStrength - schedStrength
	if diff < 0 {
		diff = -diff
	}
	return diff <= 20
}

func strengthOrDefault(signal Signal, flag Flag) int {
	if signal.Strength == nil {
		return 0
	}
	if value, ok := signal.Strength[flag]; ok {
		return value
	}
	if signal.Flags[flag] {
		return 50
	}
	return 0
}

func profileForFlag(flag Flag) CapabilityProfile {
	switch flag {
	case FlagAutomation:
		return ProfileAutomation
	case FlagScheduling:
		return ProfileScheduledReport
	case FlagBrowser:
		return ProfileBrowserAutomation
	case FlagWebLookup:
		return ProfileWebLookup
	case FlagSystemProbe:
		return ProfileSystemInspect
	case FlagCodeEdit, FlagEmail:
		return ProfileCodebaseEdit
	default:
		panic(fmt.Sprintf("unknown intent flag: %d", flag))
	}
}
