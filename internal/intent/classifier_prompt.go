package intent

const classifierPrompt = `You classify user messages into capability profiles. Return pure JSON only.

Profiles:
- "automation": user wants persistent monitoring, watchers, auto-remediation (monitor, watch, on failure, incident)
- "scheduled_report": user wants a delayed or recurring report/reminder (every day, cron, tomorrow, remind me)
- "browser_automation": user wants interactive webpage actions (click, login, screenshot, fill form + URL/website)
- "web_lookup": user wants to read or summarize a webpage, news, or weather
- "system_inspect": user wants to check local system state (version, process, port, environment)
- "codebase_edit": everything else, including code changes, bug fixes, and explanations

If automation and scheduling are both plausible, return {"profile":"automation","fallback_profile":"scheduled_report","needs_confirm":true,"confirm_text":"..."}.

"email" or "notify" are delivery methods, not profiles. "monitor files and email me" is automation.

Return JSON like {"profile":"<name>","fallback_profile":"<optional>","needs_confirm":false,"modifiers":["email"]}.`
