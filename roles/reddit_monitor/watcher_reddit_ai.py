#!/usr/bin/env python3
"""Reddit AI 监控 watcher 信号
触发状态：每天一次的稳定 dedupe_key (reddit-ai-daily:YYYY-MM-DD)
实际抓取和报告生成在 owner task 中执行。
"""
import json
from datetime import date, datetime, timezone

today = date.today()
dedupe_key = f"reddit-ai-daily:{today.isoformat()}"

print(json.dumps({
    "status": "needs_agent",
    "summary": f"Reddit AI 每日监控触发 — {today.isoformat()}",
    "dedupe_key": dedupe_key,
    "facts": {
        "trigger_time_utc": datetime.now(timezone.utc).isoformat(),
        "trigger_date": today.isoformat(),
        "subreddits": [
            "r/artificial",
            "r/singularity",
            "r/LocalLLaMA",
            "r/ChatGPT",
            "r/MachineLearning"
        ]
    }
}))
