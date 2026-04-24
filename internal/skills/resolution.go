package skills

type Resolution struct {
	Activated []ActivatedSkill
}

func Resolve(message string, catalog Catalog, inherited []string) Resolution {
	_ = message
	return Resolution{
		Activated: SelectByNames(catalog, inherited, ActivationReasonInherited),
	}
}
