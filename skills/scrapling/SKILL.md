---
name: scrapling
description: Undetectable HTTP web scraping with browser fingerprint impersonation
triggers:
  - "scrape"
  - "web scrape"
  - "fetch page"
  - "scrapling"
allowed_tools:
  - shell
  - file_read
  - file_write
policy:
  allow_implicit_activation: true
  capability_tags:
    - scraping
    - http
    - web
---

# Scrapling Skill

Scrapling 是一个无检测的 HTTP 抓取库，通过模拟浏览器指纹来绕过反爬。

## 基础用法（scrapling >= 0.4.x）

### 1. Fetcher - 抓取页面

```python
from scrapling import Fetcher

f = Fetcher()
resp = f.get('https://old.reddit.com/r/artificial/hot/?limit=5',
             impersonate='chrome',
             timeout=30)

# 检查状态码
print(resp.status)

# 如果 403 则等待后重试
if resp.status == 403:
    import time
    time.sleep(60)
    resp = f.get(url, impersonate='chrome', timeout=30)
```

### 2. 解析 HTML

⚠️ **注意**：在 scrapling >= 0.4.x 中，`f.fetch()` 已废弃，`resp.html` 属性已移除。
Response 对象本身就是可查询的 HTML 对象，直接用 `.css()` 查询：

```python
# ✅ 正确用法：直接在 resp 上调用 .css()
titles = resp.css('a.title')  # 返回列表
for t in titles:
    print(t.text, t.attrib.get('href'))

# 提取单个元素
first_title = resp.css('a.title').first
if first_title:
    print(first_title.text)

# 提取属性/文本
for link in resp.css('a.title'):
    title_text = link.text
    href = link.attrib.get('href', '')

# 获取原始 HTML 文本
html_text = resp.html_content

# XPath 支持
items = resp.xpath('//div[@class="thing"]')
```

### 3. 完整抓取示例（old.reddit.com HTML 方案）

```python
from scrapling import Fetcher
import time

def scrape_subreddit(subreddit: str) -> list:
    """抓取单个子版块热帖"""
    f = Fetcher()
    url = f'https://old.reddit.com/r/{subreddit}/hot/?limit=5'
    resp = f.get(url, impersonate='chrome', timeout=30)

    if resp.status != 200:
        if resp.status == 429:
            time.sleep(120)
            resp = f.get(url, impersonate='chrome', timeout=30)
        elif resp.status == 403:
            time.sleep(60)
            resp = f.get(url, impersonate='chrome', timeout=30)
        if resp.status != 200:
            return []

    # ⚡ resp 直接可查询
    posts = []
    for thing in resp.css('.thing'):
        title_el = thing.css('a.title').first
        if not title_el:
            continue
        posts.append({
            'title': title_el.text.strip(),
            'url': title_el.attrib.get('href', ''),
            'score': thing.css('.score.unvoted').first.text.strip() if thing.css('.score.unvoted').first else '0',
            'comments': thing.css('a.comments').first.text.strip() if thing.css('a.comments').first else '0',
            'author': thing.css('a.author').first.text.strip() if thing.css('a.author').first else 'unknown',
            'subreddit': subreddit,
        })
    return posts
```

## 注意事项

- 当前安装版本：**scrapling 0.4.7**
- `Fetcher()` 实例化时可能有 deprecation warning，不影响使用
- 可调用 `Fetcher.configure()` 进行全局配置（可选）
- 方法列表：`.get()` `.post()` `.put()` `.delete()` `.adaptive()`

## 速率限制

- 每次 HTTP 请求后必须 `time.sleep(10)`
- 429 状态码：额外等待 120 秒
- 403 状态码：额外等待 60 秒

## 参考

- 使用 `old.reddit.com` 域名（简洁 HTML，无需 JavaScript）
- `impersonate='chrome'` 模拟 Chrome 浏览器指纹
- 无需浏览器/Puppeteer/Selenium
