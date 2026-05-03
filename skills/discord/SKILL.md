---
name: discord
description: Send messages to Discord channels via webhooks
triggers:
  - "discord"
  - "send to discord"
  - "discord webhook"
  - "notify discord"
  - "post to discord"
allowed_tools:
  - shell
policy:
  allow_implicit_activation: true
  capability_tags:
    - discord
    - messaging
    - notification
---

# Discord Skill

Send messages to Discord channels using webhooks.

## Prerequisites

A Discord webhook URL must be configured. The agent checks:

1. Environment variable `DISCORD_WEBHOOK_URL`
2. File `.sesame/discord_webhook` in the workspace

## Sending a message

```bash
WEBHOOK_URL="${DISCORD_WEBHOOK_URL:-$(cat .sesame/discord_webhook 2>/dev/null)}"
if [ -z "$WEBHOOK_URL" ]; then
  echo "Error: No Discord webhook URL configured."
  echo "Set DISCORD_WEBHOOK_URL env var or create .sesame/discord_webhook file."
  exit 1
fi

# Simple text message
curl -s -H "Content-Type: application/json" \
  -d '{"content":"Your message here"}' \
  "$WEBHOOK_URL"

# Rich embed message
curl -s -H "Content-Type: application/json" \
  -d '{
    "embeds": [{
      "title": "Report Title",
      "description": "Report description here",
      "color": 5814783,
      "timestamp": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"
    }]
  }' \
  "$WEBHOOK_URL"
```

## Guidelines

- Discord messages have a 2000 character limit. Split long messages.
- Use embeds for structured reports, plain text for quick notifications.
- Do not spam; batch multiple notifications when possible.
