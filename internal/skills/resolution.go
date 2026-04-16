package skills

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

func Resolve(message string, catalog Catalog, inherited []string) Resolution {
	activated := MergeActivatedSkills(
		Activate(catalog, message),
		SelectByNames(catalog, inherited, ActivationReasonInherited),
	)
	return Resolution{Activated: activated}
}
