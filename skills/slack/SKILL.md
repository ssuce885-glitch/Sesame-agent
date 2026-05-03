---
name: slack
description: Send messages to Slack channels via incoming webhooks
triggers:
  - "slack"
  - "send to slack"
  - "notify slack"
  - "post to slack"
allowed_tools:
  - shell
policy:
  allow_implicit_activation: true
  capability_tags:
    - slack
    - messaging
    - notification
---

# Slack Skill

Send messages to Slack channels using incoming webhooks.

## Prerequisites

A Slack webhook URL must be configured. Check:

1. Environment variable `SLACK_WEBHOOK_URL`
2. File `.sesame/slack_webhook` in the workspace

## Sending messages

```bash
WEBHOOK_URL="${SLACK_WEBHOOK_URL:-$(cat .sesame/slack_webhook 2>/dev/null)}"
if [ -z "$WEBHOOK_URL" ]; then
  echo "Error: No Slack webhook URL configured."
  echo "Set SLACK_WEBHOOK_URL env var or create .sesame/slack_webhook file."
  exit 1
fi

# Plain text
curl -s -X POST -H "Content-Type: application/json" \
  -d '{"text":"Your message here"}' \
  "$WEBHOOK_URL"

# With attachments
curl -s -X POST -H "Content-Type: application/json" \
  -d '{
    "text": "Report",
    "attachments": [{
      "title": "Report Title",
      "text": "Detailed report content",
      "color": "#36a64f"
    }]
  }' \
  "$WEBHOOK_URL"

# Markdown blocks
curl -s -X POST -H "Content-Type: application/json" \
  -d '{
    "blocks": [
      {"type": "header", "text": {"type": "plain_text", "text": "Report"}},
      {"type": "section", "text": {"type": "mrkdwn", "text": "*Status*: Complete\n*Items*: 5 processed"}}
    ]
  }' \
  "$WEBHOOK_URL"
```

## Guidelines

- Use Block Kit format for rich messages when available.
- Keep messages under Slack's 40000 character limit.
- Batch notifications when possible.
