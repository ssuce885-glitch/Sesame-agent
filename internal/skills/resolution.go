package skills

type Resolution struct {
	Activated []ActivatedSkill
}

func Resolve(message string, catalog Catalog, inherited []string) Resolution {
	activated := MergeActivatedSkills(
		Activate(catalog, message),
		SelectByNames(catalog, inherited, ActivationReasonInherited),
	)
	retrieval := Retrieve(catalog, message, activated)
	activated = MergeActivatedSkills(activated, retrieval.Selected)
	return Resolution{Activated: activated}
}
