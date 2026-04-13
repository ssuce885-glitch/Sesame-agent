package intent

import (
	"regexp"
	"strings"
)

type Flag int

const (
	FlagAutomation Flag = iota
	FlagScheduling
	FlagBrowser
	FlagWebLookup
	FlagSystemProbe
	FlagCodeEdit
	FlagEmail
)

type Signal struct {
	Raw            string
	Flags          map[Flag]bool
	Strength       map[Flag]int
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

var (
	skillRefPattern         = regexp.MustCompile(`\$([A-Za-z0-9._-]+)`)
	englishDelayPattern     = regexp.MustCompile(`\bin\s+\d+\s+(minute|minutes|hour|hours|day|days|week|weeks|month|months)\b`)
	englishRecurringPattern = regexp.MustCompile(`\bevery\s+(day|week|month|hour|morning|evening)\b`)
)

func Scan(userMessage string, catalog SkillCatalog) Signal {
	raw := strings.TrimSpace(userMessage)
	signal := Signal{
		Raw:      raw,
		Flags:    make(map[Flag]bool),
		Strength: make(map[Flag]int),
	}
	if raw == "" {
		signal.Flags[FlagCodeEdit] = true
		signal.Strength[FlagCodeEdit] = 1
		return signal
	}

	text := strings.ToLower(raw)
	signal.Strength[FlagAutomation] = detectStrength(text, []string{
		"自动", "异常时", "失败时", "监控", "巡检", "唤起代理",
		"automatically", "on failure", "monitor", "watch", "poll", "incident",
	}, 30)
	signal.Strength[FlagScheduling] = detectStrength(text, []string{
		"分钟后", "小时后", "天后", "稍后", "定时", "每天", "每周", "每月", "cron",
		"later", "tomorrow", "every day", "every week", "every month",
	}, 20)
	if englishDelayPattern.MatchString(text) || englishRecurringPattern.MatchString(text) {
		signal.Strength[FlagScheduling] += 20
	}
	signal.Strength[FlagBrowser] = detectStrength(text, []string{
		"打开", "点击", "登录", "截图", "网页", "网站",
		"open", "click", "login", "sign in", "screenshot", "website", "browser",
	}, 20)
	signal.Strength[FlagWebLookup] = detectStrength(text, []string{
		"天气", "新闻", "网页", "网站", "http://", "https://",
		"weather", "news", "headline", "url", "web page", "website",
	}, 15)
	signal.Strength[FlagSystemProbe] = detectStrength(text, []string{
		"系统", "环境", "版本", "进程", "端口", "路径",
		"system", "environment", "version", "process", "port", "which ",
	}, 15)
	signal.Strength[FlagEmail] = detectStrength(text, []string{
		"邮箱", "邮件", "发邮件", "email", "mail", "send mail",
	}, 20)
	signal.Strength[FlagCodeEdit] = detectStrength(text, []string{
		"脚本", "代码", "函数", "修复", "实现",
		"script", "code", "function", "fix", "implement",
	}, 10)

	for flag, score := range signal.Strength {
		if score > 0 {
			signal.Flags[flag] = true
		}
	}
	if len(signal.Flags) == 0 {
		signal.Flags[FlagCodeEdit] = true
		signal.Strength[FlagCodeEdit] = 1
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

func detectStrength(text string, markers []string, weight int) int {
	score := 0
	for _, marker := range markers {
		if marker != "" && strings.Contains(text, marker) {
			score += weight
		}
	}
	return score
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
