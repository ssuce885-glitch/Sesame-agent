package skills

import (
	"sort"
	"strings"
)

type TaskUnderstanding struct {
	NormalizedMessage      string
	WantsScheduling        bool
	WantsBrowserAutomation bool
	WantsWebLookup         bool
	WantsEmailDelivery     bool
	WantsSystemInspect     bool
	WantsCodeEdit          bool
}

type RetrievalCandidate struct {
	Skill        SkillSpec
	Score        int
	Reasons      []string
	AutoActivate bool
}

type RetrievalResult struct {
	Task      TaskUnderstanding
	Selected  []ActivatedSkill
	Suggested []RetrievalCandidate
}

type skillTraits struct {
	BrowserAutomation bool
	EmailDelivery     bool
	SystemInspect     bool
	CodeEdit          bool
	WebLookup         bool
	GrantsShell       bool
}

func Retrieve(catalog Catalog, userMessage string, already []ActivatedSkill) RetrievalResult {
	task := UnderstandTask(userMessage)
	return retrieveForTask(catalog, task, already)
}

func RetrieveForExecution(catalog Catalog, userMessage string, already []ActivatedSkill) RetrievalResult {
	task := UnderstandTask(userMessage)
	task.WantsScheduling = false
	return retrieveForTask(catalog, task, already)
}

func retrieveForTask(catalog Catalog, task TaskUnderstanding, already []ActivatedSkill) RetrievalResult {
	candidates := rankRetrievalCandidates(catalog, task, already)
	if len(candidates) == 0 {
		return RetrievalResult{Task: task}
	}

	if task.WantsScheduling {
		return RetrievalResult{Task: task}
	}

	selected := make([]ActivatedSkill, 0, 3)
	suggested := make([]RetrievalCandidate, 0, 3)
	for _, candidate := range candidates {
		if candidate.AutoActivate && len(selected) < 3 {
			selected = append(selected, ActivatedSkill{
				Skill:       candidate.Skill,
				Reason:      ActivationReasonRetrieved,
				MatchedText: strings.Join(candidate.Reasons, ", "),
			})
			continue
		}
		if len(suggested) < 3 {
			suggested = append(suggested, candidate)
		}
	}

	return RetrievalResult{
		Task:      task,
		Selected:  selected,
		Suggested: suggested,
	}
}

func UnderstandTask(userMessage string) TaskUnderstanding {
	text := normalizeSkillMatchText(userMessage)
	if text == "" {
		return TaskUnderstanding{}
	}

	hasDelayed := containsAny(text, []string{
		"分钟后", "小时后", "天后", "稍后", "晚点", "定时", "cron",
		"later", "tomorrow", "in 5 minutes", "in 10 minutes", "in two minutes",
	})
	hasRecurring := containsAny(text, []string{
		"每天", "每周", "每月", "日报", "周报", "月报", "巡检", "周期",
		"every day", "every week", "every month", "recurring", "daily report",
	})
	hasDelivery := containsAny(text, []string{
		"告诉我", "给我", "汇报", "报告", "提醒我", "通知我", "发送", "发给", "发送到",
		"tell me", "report", "notify me", "remind me", "send to", "send ",
	})
	wantsEmail := containsAny(text, []string{
		"邮件", "邮箱", "发邮件", "email", "mail", "smtp",
	})
	wantsBrowser := containsAny(text, []string{
		"打开", "点击", "登录", "登上", "截图", "表单", "填写", "提交", "上传", "下载页面",
		"open", "click", "log in", "login", "sign in", "screenshot", "form", "submit", "fill",
	}) && containsAny(text, []string{
		"http://", "https://", "网址", "网站", "网页", "页面",
		"url", "site", "website", "web page", "page", "browser",
	})

	wantsScheduling := hasRecurring || (hasDelayed && (hasDelivery || wantsEmail))
	wantsWebLookup := containsAny(text, []string{
		"http://", "https://", "网址", "网站", "网页", "页面摘要", "网页总结",
		"新闻", "热门新闻", "热点", "天气", "股价", "汇率",
		"url", "website", "web page", "news", "headline", "weather", "web summary",
	})
	wantsSystemInspect := containsAny(text, []string{
		"系统", "环境", "版本", "路径", "进程", "端口", "wsl", "which ",
		"system", "environment", "version", "binary", "process", "port", "which ", "uname",
	})
	wantsCodeEdit := containsAny(text, []string{
		"代码", "改代码", "修改代码", "重构", "补丁", "实现", "修复", "review", "审查", "重写",
		"code", "refactor", "patch", "implement", "fix", "rewrite",
	})

	return TaskUnderstanding{
		NormalizedMessage:      text,
		WantsScheduling:        wantsScheduling,
		WantsBrowserAutomation: wantsBrowser,
		WantsWebLookup:         wantsWebLookup,
		WantsEmailDelivery:     wantsEmail,
		WantsSystemInspect:     wantsSystemInspect,
		WantsCodeEdit:          wantsCodeEdit,
	}
}

func rankRetrievalCandidates(catalog Catalog, task TaskUnderstanding, already []ActivatedSkill) []RetrievalCandidate {
	if len(catalog.Skills) == 0 || strings.TrimSpace(task.NormalizedMessage) == "" {
		return nil
	}

	activeNames := make(map[string]struct{}, len(already))
	for _, item := range already {
		key := strings.ToLower(strings.TrimSpace(item.Skill.Name))
		if key == "" {
			continue
		}
		activeNames[key] = struct{}{}
	}

	out := make([]RetrievalCandidate, 0, len(catalog.Skills))
	for _, skill := range catalog.Skills {
		key := strings.ToLower(strings.TrimSpace(skill.Name))
		if key == "" {
			continue
		}
		if _, ok := activeNames[key]; ok {
			continue
		}
		score, reasons := scoreRetrievalCandidate(task, skill)
		if score <= 0 {
			continue
		}
		candidate := RetrievalCandidate{
			Skill:   skill,
			Score:   score,
			Reasons: reasons,
		}
		candidate.AutoActivate = shouldAutoActivateCandidate(task, candidate)
		out = append(out, candidate)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			left := strings.ToLower(strings.TrimSpace(out[i].Skill.Name))
			right := strings.ToLower(strings.TrimSpace(out[j].Skill.Name))
			return left < right
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func scoreRetrievalCandidate(task TaskUnderstanding, skill SkillSpec) (int, []string) {
	traits := inferSkillTraits(skill)
	reasons := make([]string, 0, 4)
	score := 0
	text := skillSearchText(skill)

	if normalizedName := normalizeSkillMatchText(skill.Name); normalizedName != "" && strings.Contains(task.NormalizedMessage, normalizedName) {
		score += 120
		reasons = append(reasons, "skill name mentioned")
	}
	for _, trigger := range skill.Triggers {
		normalizedTrigger := normalizeSkillMatchText(trigger)
		if normalizedTrigger == "" || !strings.Contains(task.NormalizedMessage, normalizedTrigger) {
			continue
		}
		score += 80
		reasons = append(reasons, "trigger matched")
		break
	}

	if task.WantsBrowserAutomation {
		if traits.BrowserAutomation {
			score += 110
			reasons = append(reasons, "browser automation task")
		}
	} else if traits.BrowserAutomation && (task.WantsWebLookup || task.WantsEmailDelivery || task.WantsSystemInspect) {
		score -= 140
	}

	if task.WantsEmailDelivery {
		if traits.EmailDelivery {
			score += 120
			reasons = append(reasons, "email delivery task")
		}
		if traits.GrantsShell {
			score += 10
		}
	} else if traits.EmailDelivery {
		score -= 40
	}

	if task.WantsSystemInspect {
		if traits.SystemInspect {
			score += 90
			reasons = append(reasons, "system inspection task")
		}
		if traits.GrantsShell {
			score += 10
		}
	} else if traits.SystemInspect {
		score -= 20
	}

	if task.WantsCodeEdit {
		if traits.CodeEdit {
			score += 80
			reasons = append(reasons, "code task")
		}
	} else if traits.CodeEdit {
		score -= 20
	}

	if task.WantsWebLookup && traits.WebLookup && !traits.BrowserAutomation {
		score += 20
	}

	if strings.Contains(text, "shell_command") && (task.WantsBrowserAutomation || task.WantsEmailDelivery || task.WantsSystemInspect) {
		score += 10
	}

	return score, reasons
}

func shouldAutoActivateCandidate(task TaskUnderstanding, candidate RetrievalCandidate) bool {
	if task.WantsScheduling || candidate.Score < 80 {
		return false
	}
	traits := inferSkillTraits(candidate.Skill)
	switch {
	case traits.EmailDelivery && task.WantsEmailDelivery:
		return true
	case traits.SystemInspect && task.WantsSystemInspect:
		return true
	case traits.BrowserAutomation && task.WantsBrowserAutomation:
		return true
	case traits.CodeEdit && task.WantsCodeEdit && candidate.Score >= 100:
		return true
	default:
		return candidate.Score >= 140
	}
}

func inferSkillTraits(skill SkillSpec) skillTraits {
	text := skillSearchText(skill)
	grantsShell := false
	for _, toolName := range append(append([]string(nil), skill.AllowedTools...), skill.Agent.Tools...) {
		if strings.EqualFold(strings.TrimSpace(toolName), "shell_command") {
			grantsShell = true
			break
		}
	}
	if !grantsShell {
		for _, toolName := range skill.Policy.PreferredTools {
			if strings.EqualFold(strings.TrimSpace(toolName), "shell_command") {
				grantsShell = true
				break
			}
		}
	}

	return skillTraits{
		BrowserAutomation: hasCapabilityTag(skill, "browser_automation") || hasBrowserAutomationSignal(text),
		EmailDelivery: containsAny(text, []string{
			"send-email", "email", "smtp", "邮件", "邮箱",
		}) || containsEnglishToken(text, "mail"),
		SystemInspect: containsAny(text, []string{
			"system", "environment", "version", "binary", "process", "wsl", "which ", "系统", "环境", "版本", "路径",
		}),
		CodeEdit: containsAny(text, []string{
			"edit code", "patch", "refactor", "rewrite code", "修改代码", "补丁", "重构", "代码",
		}),
		WebLookup: containsAny(text, []string{
			"news", "weather", "headline", "web page", "web summary", "新闻", "天气", "网页摘要", "网页总结",
		}),
		GrantsShell: grantsShell,
	}
}

func hasBrowserAutomationSignal(text string) bool {
	if containsAny(text, []string{
		"browser", "playwright", "chromium", "website", "web page", "site", "url",
		"网站", "网页", "页面", "截图",
	}) {
		return true
	}
	return containsAny(text, []string{
		"click", "log in", "login to", "sign in", "form", "submit",
		"点击", "登录", "表单", "填写", "提交",
	}) && containsAny(text, []string{
		"browser", "playwright", "chromium", "website", "web page", "site", "url", "page",
		"网站", "网页", "页面",
	})
}

func hasCapabilityTag(skill SkillSpec, tag string) bool {
	want := strings.ToLower(strings.TrimSpace(tag))
	if want == "" {
		return false
	}
	for _, item := range skill.Policy.CapabilityTags {
		if strings.ToLower(strings.TrimSpace(item)) == want {
			return true
		}
	}
	return false
}

func skillSearchText(skill SkillSpec) string {
	parts := []string{
		skill.Name,
		skill.Description,
		skill.Body,
		skill.Agent.Description,
		skill.Agent.Instructions,
	}
	parts = append(parts, skill.Triggers...)
	parts = append(parts, skill.Policy.CapabilityTags...)
	parts = append(parts, skill.Policy.PreferredTools...)
	parts = append(parts, skill.AllowedTools...)
	parts = append(parts, skill.Agent.Tools...)
	return strings.ToLower(strings.TrimSpace(strings.Join(parts, "\n")))
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func containsEnglishToken(text, token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return false
	}
	for _, field := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if field == token {
			return true
		}
	}
	return false
}
