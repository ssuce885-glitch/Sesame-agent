---
name: email
description: Send email notifications and reports via sendmail, SMTP, or email APIs
triggers:
  - "send email"
  - "email report"
  - "notify via email"
  - "mail to"
allowed_tools:
  - shell
  - file_read
policy:
  allow_implicit_activation: true
  capability_tags:
    - email
    - notification
    - messaging
---

# Email Skill

You can send emails from the workspace using available system tools.

## Method 1: sendmail (preferred if installed)

```bash
cat <<'EOF' | sendmail -t
To: recipient@example.com
Subject: Report from Sesame
Content-Type: text/plain; charset=utf-8

Email body text here.
EOF
```

## Method 2: curl + Mailgun API

```bash
curl -s --user "api:${MAILGUN_API_KEY}" \
  "https://api.mailgun.net/v3/${MAILGUN_DOMAIN}/messages" \
  -F from="Sesame <sesame@${MAILGUN_DOMAIN}>" \
  -F to="recipient@example.com" \
  -F subject="Report from Sesame" \
  -F text="Email body text here."
```

## Method 3: curl + SendGrid API

```bash
curl -s -X POST "https://api.sendgrid.com/v3/mail/send" \
  -H "Authorization: Bearer ${SENDGRID_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "personalizations": [{"to": [{"email": "recipient@example.com"}]}],
    "from": {"email": "sender@example.com", "name": "Sesame"},
    "subject": "Report from Sesame",
    "content": [{"type": "text/plain", "value": "Email body text here."}]
  }'
```

## Method 4: Python smtplib

```python
import smtplib
from email.mime.text import MIMEText

msg = MIMEText("Email body text here.")
msg["Subject"] = "Report from Sesame"
msg["From"] = "sender@example.com"
msg["To"] = "recipient@example.com"

with smtplib.SMTP("smtp.example.com", 587) as server:
    server.starttls()
    server.login("user", "password")
    server.send_message(msg)
```

## Guidelines

- Check which method is available before sending (sendmail > curl API > python)
- Default recipients should come from workspace settings or be asked from the user
- Email content should be plain text unless HTML is specifically requested
- Always confirm before sending to external recipients
- Respect rate limits; don't send more than 10 emails per hour unless explicitly authorized
