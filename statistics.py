import os
import re
import sys
import time
from collections import defaultdict
from typing import Dict
from fastapi import Request

SHOW_ORIGINAL_LOGS = os.getenv("SHOW_LOG", "0") == "1"

visit_count: Dict[str, int] = defaultdict(int)  # IP -> 打开次数
ip_seen_titles: Dict[str, set] = defaultdict(set)  # IP -> 已访问标题集


def log_request(ip: str, path: str, html: str):
    normalized = path.lstrip("/")
    if not normalized.startswith("g/"):
        return

    # 提取标题
    m = re.search(r'<h1\s+id=["\']gn["\']\s*>\s*(.*?)\s*</h1>', html,
                  re.DOTALL | re.IGNORECASE)
    if not m:
        return
    title = m.group(1).strip()

    # 只统计同一个 IP 对同一标题第一次访问
    if title not in ip_seen_titles[ip]:
        ip_seen_titles[ip].add(title)
        visit_count[ip] += 1

        print(f"{ip}: {visit_count[ip]} - {title}")


def patch_logging(app):
    if not SHOW_ORIGINAL_LOGS:
        import logging
        logging.getLogger("uvicorn").setLevel(logging.ERROR)
        logging.getLogger("uvicorn.access").setLevel(logging.ERROR)
        logging.getLogger("uvicorn.error").setLevel(logging.ERROR)
        print("已禁用原始 FastAPI 输出")


async def track_request(request: Request, html: str):
    client_ip = request.client.host if request.client else "unknown"
    path = request.url.path
    log_request(client_ip, path, html)
 # type: ignore