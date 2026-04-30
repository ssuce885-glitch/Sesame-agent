#!/usr/bin/env python3
"""Reddit AI 监控 — 抓取 5 个子版块 hot 前 5 篇帖子"""
import time
from scrapling.fetchers import Fetcher

SUBREDDITS = [
    "artificial",
    "singularity",
    "LocalLLaMA",
    "ChatGPT",
    "MachineLearning",
]

def fetch_subreddit(name, max_retries=2):
    """抓取单个子版块，遇到 403 等待 60s 重试"""
    url = f"https://old.reddit.com/r/{name}/hot/?limit=5"
    for attempt in range(1 + max_retries):
        try:
            page = Fetcher.get(url, impersonate='chrome', timeout=45)
            if page.status == 200:
                return page
            elif page.status == 403:
                if attempt < max_retries:
                    print(f"  ⚠️  r/{name} 403，等待 60 秒后重试 ({attempt+1}/{max_retries})...")
                    time.sleep(60)
                    continue
                else:
                    print(f"  ❌ r/{name} 403，重试耗尽，跳过")
                    return None
            else:
                print(f"  ⚠️  r/{name} HTTP {page.status}，跳过")
                return None
        except Exception as e:
            if attempt < max_retries:
                print(f"  ⚠️  r/{name} 出错: {e}，60 秒后重试 ({attempt+1}/{max_retries})...")
                time.sleep(60)
                continue
            else:
                print(f"  ❌ r/{name} 异常: {e}，跳过")
                return None
    return None

def parse_post(entry):
    """从帖子条目提取信息"""
    title_texts = entry.css("a.title::text").getall()
    title = ""
    for t in title_texts:
        t = t.strip()
        if t:
            title = t
            break
    link = entry.css("a.title::attr(href)").get() or ""
    if link.startswith("/r/"):
        link = "https://old.reddit.com" + link

    score_text = entry.css("div.score.unvoted::text").get() or "?"
    score = score_text.strip()

    comments_text = entry.css("a.comments::text").get() or "0 comments"
    comments = comments_text.strip().replace("comments", "评论").replace("comment", "评论")

    return score, title, link, comments


def main():
    import datetime
    now = datetime.datetime.now(datetime.timezone.utc)
    cn_tz = now + datetime.timedelta(hours=8)
    date_str = cn_tz.strftime("%Y-%m-%d %H:%M")

    start = time.time()
    lines = []
    lines.append(f"📡 Reddit AI 动态汇总 — {date_str}")
    lines.append("━" * 50)

    total_posts = 0

    for name in SUBREDDITS:
        lines.append("")
        lines.append(f"🔥 r/{name}")
        page = fetch_subreddit(name)
        if page is None:
            lines.append("  ❌ 抓取失败")
            continue

        entries = page.css(".thing:not(.promoted)")
        count = 0
        for entry in entries:
            if count >= 5:
                break
            cls = entry.css("::attr(class)").get() or ""
            if "stickied" in cls:
                continue
            score, title, link, comments = parse_post(entry)
            if not title:
                continue
            lines.append(f"  [{score}] {title}")
            lines.append(f"  → {link} | {comments}")
            count += 1
            total_posts += 1
        if count == 0:
            lines.append("  （暂无新帖）")

    elapsed = round(time.time() - start, 1)
    lines.append("")
    lines.append("━" * 50)
    lines.append(f"共 {total_posts} 个帖子 | 抓取耗时 {elapsed} 秒")

    print("\n".join(lines))


if __name__ == "__main__":
    main()
