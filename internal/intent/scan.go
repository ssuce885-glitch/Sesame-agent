package intent

import (
	"regexp"
	"strings"
)

type Signal struct {
	Raw            string
	ExplicitSkills []string
	NameMatches    []string
}

type SkillCatalog interface {
	SkillNames() []string
}

type SkillCatalogView struct {
	Skills []SkillNameView
}

type SkillNameView struct {
	Name string
}

func (v SkillCatalogView) SkillNames() []string {
	return namesFromView(v.Skills)
}

var skillRefPattern = regexp.MustCompile(`\$([A-Za-z0-9._-]+)`)

func ExtractSkillSignals(userMessage string, catalog SkillCatalog) Signal {
	raw := strings.TrimSpace(userMessage)
	signal := Signal{Raw: raw}
	if raw == "" {
		return signal
	}

	skillNames := []string(nil)
	if catalog != nil {
		skillNames = catalog.SkillNames()
	}
	nameByKey := make(map[string]string, len(skillNames))
	for _, name := range skillNames {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		nameByKey[key] = name
	}

	expSeen := map[string]struct{}{}
	for _, match := range skillRefPattern.FindAllStringSubmatch(raw, -1) {
		if len(match) < 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(match[1]))
		name, ok := nameByKey[key]
		if !ok {
			continue
		}
		if _, seen := expSeen[key]; seen {
			continue
		}
		expSeen[key] = struct{}{}
		signal.ExplicitSkills = append(signal.ExplicitSkills, name)
	}

	normalizedMessage := normalizeSkillText(raw)
	nameSeen := map[string]struct{}{}
	for _, name := range skillNames {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		if _, explicit := expSeen[key]; explicit {
			continue
		}
		normalizedName := normalizeSkillText(name)
		if normalizedName == "" || !strings.Contains(normalizedMessage, normalizedName) {
			continue
		}
		if _, seen := nameSeen[key]; seen {
			continue
		}
		nameSeen[key] = struct{}{}
		signal.NameMatches = append(signal.NameMatches, name)
	}

	return signal
}

// Scan is kept as a narrow compatibility wrapper while callers migrate.
func Scan(userMessage string, catalog SkillCatalog) Signal {
	return ExtractSkillSignals(userMessage, catalog)
}

func normalizeSkillText(value string) string {
	replacer := strings.NewReplacer("-", " ", "_", " ", ".", " ", "/", " ", "\\", " ")
	normalized := replacer.Replace(strings.ToLower(strings.TrimSpace(value)))
	return strings.Join(strings.Fields(normalized), " ")
}

func namesFromView(skills []SkillNameView) []string {
	if len(skills) == 0 {
		return nil
	}
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		if trimmed := strings.TrimSpace(skill.Name); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
