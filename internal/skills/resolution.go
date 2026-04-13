package skills

import (
	"strings"

	"go-agent/internal/intent"
)

type SuggestedSkill struct {
	Name        string
	Description string
	Grants      []string
	Reasons     []string
}

type Resolution struct {
	Activated []ActivatedSkill
	Suggested []SuggestedSkill
}

func Resolve(plan intent.Plan, catalog Catalog, inherited []string) Resolution {
	activated := MergeActivatedSkills(
		SelectByNames(catalog, plan.Signal.ExplicitSkills, ActivationReasonExplicit),
		SelectByNames(catalog, plan.Signal.NameMatches, ActivationReasonName),
		SelectByNames(catalog, inherited, ActivationReasonInherited),
		SelectByCapabilityTags(catalog, capabilityTagsForProfile(plan.Profile)),
	)
	return Resolution{
		Activated: activated,
		Suggested: suggestedFromPlan(plan, catalog, activated),
	}
}

func suggestedFromPlan(plan intent.Plan, catalog Catalog, activated []ActivatedSkill) []SuggestedSkill {
	retrieval := Retrieve(catalog, strings.TrimSpace(plan.Signal.Raw), activated)
	if len(retrieval.Suggested) == 0 {
		return nil
	}
	out := make([]SuggestedSkill, 0, len(retrieval.Suggested))
	for _, candidate := range retrieval.Suggested {
		out = append(out, SuggestedSkill{
			Name:        candidate.Skill.Name,
			Description: candidate.Skill.Description,
			Grants:      GrantedTools([]ActivatedSkill{{Skill: candidate.Skill, Reason: ActivationReasonToolUse}}),
			Reasons:     append([]string(nil), candidate.Reasons...),
		})
	}
	return out
}

func capabilityTagsForProfile(profile intent.CapabilityProfile) []string {
	switch profile {
	case intent.ProfileAutomation:
		return []string{"automation_standard_behavior"}
	case intent.ProfileBrowserAutomation:
		return []string{"browser_automation"}
	default:
		return nil
	}
}
