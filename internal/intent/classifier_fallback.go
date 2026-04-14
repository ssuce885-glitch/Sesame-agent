package intent

import (
	"regexp"
	"strings"
)

var (
	englishDelayPattern     = regexp.MustCompile(`\bin\s+\d+\s+(minute|minutes|hour|hours|day|days|week|weeks|month|months)\b`)
	englishRecurringPattern = regexp.MustCompile(`\bevery\s+(day|week|month|hour|morning|evening)\b`)
)

func FallbackClassify(message string) ClassifierResult {
	text := strings.ToLower(strings.TrimSpace(message))
	modifiers := detectModifiers(text)
	if text == "" {
		return ClassifierResult{Profile: ProfileCodebaseEdit, Modifiers: modifiers}
	}

	hasAutomation := isAutomationRequest(text)
	hasScheduling := isScheduledReportRequest(text)
	switch {
	case hasAutomation && hasScheduling:
		return normalizeClassifierResult(ClassifierResult{
			Profile:         ProfileAutomation,
			FallbackProfile: ProfileScheduledReport,
			NeedsConfirm:    true,
			ConfirmText:     defaultConfirmationText,
			Modifiers:       modifiers,
		})
	case hasAutomation:
		return normalizeClassifierResult(ClassifierResult{
			Profile:   ProfileAutomation,
			Modifiers: modifiers,
		})
	case hasScheduling:
		return normalizeClassifierResult(ClassifierResult{
			Profile:   ProfileScheduledReport,
			Modifiers: modifiers,
		})
	case isBrowserAutomationRequest(text):
		return normalizeClassifierResult(ClassifierResult{
			Profile:   ProfileBrowserAutomation,
			Modifiers: modifiers,
		})
	case isWebLookupRequest(text):
		return normalizeClassifierResult(ClassifierResult{
			Profile:   ProfileWebLookup,
			Modifiers: modifiers,
		})
	case isSystemInspectRequest(text):
		return normalizeClassifierResult(ClassifierResult{
			Profile:   ProfileSystemInspect,
			Modifiers: modifiers,
		})
	default:
		return normalizeClassifierResult(ClassifierResult{
			Profile:   ProfileCodebaseEdit,
			Modifiers: modifiers,
		})
	}
}

func isAutomationRequest(text string) bool {
	longRunningMarkers := []string{
		"持续", "一直", "盯着", "盯住", "监控", "监测", "巡检", "周期检查", "定期检查", "异常时", "出问题时", "失败时",
		"watch", "monitor", "keep an eye on", "poll", "heartbeat", "on error", "on failure", "when it fails",
	}
	automationActionMarkers := []string{
		"自动排查", "自动处理", "自动通知", "自动重启", "自动修复",
		"automatically investigate", "automatically notify", "automatically restart", "auto-remediate",
	}
	targetMarkers := []string{
		"服务器", "服务", "服务状态", "异常", "容器", "任务失败", "日志", "文件", "目录", "进程", "端口",
		"server", "service", "incident", "container", "logs", "error", "failure", "file", "files", "directory", "folder", "process", "port",
	}

	hasLongRunning := containsAny(text, longRunningMarkers)
	hasAutomationAction := containsAny(text, automationActionMarkers)
	hasTarget := containsAny(text, targetMarkers)
	return hasTarget && (hasLongRunning || hasAutomationAction)
}

func isScheduledReportRequest(text string) bool {
	delayedMarkers := []string{
		"分钟后", "小时后", "天后", "稍后", "晚点", "定时", "cron",
		"later", "tomorrow", "in 5 minutes", "in 10 minutes", "in two minutes",
	}
	recurringMarkers := []string{
		"每天", "每周", "每月", "日报", "周报", "月报", "周期",
		"every day", "every week", "every month", "recurring", "daily report",
	}
	deliveryMarkers := []string{
		"告诉我", "给我", "汇报", "报告", "提醒我", "通知我",
		"tell me", "report", "notify me", "remind me",
	}

	hasDelayed := containsAny(text, delayedMarkers) || englishDelayPattern.MatchString(text)
	hasRecurring := containsAny(text, recurringMarkers) || englishRecurringPattern.MatchString(text)
	hasDelivery := containsAny(text, deliveryMarkers)
	if hasRecurring {
		return true
	}
	if hasDelayed && hasDelivery {
		return true
	}
	return hasDelayed && containsAny(text, []string{"天气", "weather"})
}

func isBrowserAutomationRequest(text string) bool {
	actionMarkers := []string{
		"打开", "点击", "登录", "登上", "截图", "表单", "填写", "提交", "上传", "下载页面",
		"open", "click", "log in", "login", "sign in", "screenshot", "form", "submit", "fill",
	}
	webMarkers := []string{
		"http://", "https://", "网址", "网站", "网页", "页面",
		"url", "site", "website", "web page", "page", "browser",
	}
	return containsAny(text, actionMarkers) && containsAny(text, webMarkers)
}

func isWebLookupRequest(text string) bool {
	webLookupMarkers := []string{
		"http://", "https://", "网址", "网站", "网页", "页面摘要", "网页总结",
		"新闻", "热门新闻", "热点", "天气", "股价", "汇率",
		"url", "website", "web page", "news", "headline", "weather", "web summary",
	}
	return containsAny(text, webLookupMarkers)
}

func isSystemInspectRequest(text string) bool {
	systemMarkers := []string{
		"系统", "环境", "版本", "路径", "进程", "端口", "wsl", "which ",
		"system", "environment", "version", "binary", "process", "port", "which ", "uname",
	}
	return containsAny(text, systemMarkers)
}

func detectModifiers(text string) []string {
	if containsAny(text, []string{"邮箱", "邮件", "发邮件", "email", "mail", "send mail", "notify"}) {
		return []string{"email"}
	}
	return nil
}

func containsAny(text string, candidates []string) bool {
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(text, candidate) {
			return true
		}
	}
	return false
}
