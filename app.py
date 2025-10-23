import os
import re
from typing import Dict
from dotenv import load_dotenv
import httpx
from fastapi import FastAPI, Request, Response
from fastapi.responses import StreamingResponse, PlainTextResponse
from statistics import patch_logging, track_request

load_dotenv()

# 默认屏蔽路径
DEFAULT_BLOCKED_PATHS = [
    "archiver.php", "mytags", "repo/torrent_post.php", "gallerytorrents.php",
    "uconfig.php", "favorites.php"
]
# 默认屏蔽查询参数
DEFAULT_BLOCKED_QUERY_KEYS = ["report", "act", "inline_set"]
# 屏蔽的 api.php 方法
BLOCKED_METHODS = ["rategallery", "votecomment", "favorite", "taggallery"]

EX_SITE = "https://exhentai.org"
IGNEOUS_FILE = ".igneous"
RAW_COOKIES = os.getenv("COOKIES", "").strip().strip("'\"")

BLOCKED_PATHS = os.getenv("BLOCKED_PATHS")
if BLOCKED_PATHS:
    BLOCKED_PATHS = [
        p.strip().lstrip("/") for p in BLOCKED_PATHS.split(",") if p.strip()
    ]
else:
    BLOCKED_PATHS = DEFAULT_BLOCKED_PATHS

BLOCKED_QUERY_KEYS = os.getenv("BLOCKED_QUERY_KEYS")
if BLOCKED_QUERY_KEYS:
    BLOCKED_QUERY_KEYS = [
        q.strip().lower() for q in BLOCKED_QUERY_KEYS.split(",") if q.strip()
    ]
else:
    BLOCKED_QUERY_KEYS = DEFAULT_BLOCKED_QUERY_KEYS

BLOCKED_METHODS = os.getenv("BLOCKED_METHODS")
if BLOCKED_METHODS:
    BLOCKED_METHODS = [
        m.strip() for m in BLOCKED_METHODS.split(",") if m.strip()
    ]
else:
    BLOCKED_METHODS = BLOCKED_METHODS

app = FastAPI()
patch_logging(app)

client = httpx.AsyncClient(follow_redirects=True, timeout=30.0)


def parse_cookie_string(s: str) -> Dict[str, str]:
    cookies = {}
    for part in s.split(";"):
        part = part.strip()
        if not part or "=" not in part:
            continue
        k, v = part.split("=", 1)
        cookies[k.strip()] = v.strip()
    return cookies


# 每次启动获取 igneous
async def fetch_igneous(cookies: Dict[str, str]) -> str | None:
    jar = httpx.Cookies()
    for k, v in cookies.items():
        if k.lower() == "igneous":
            continue
        jar.set(k, v, domain="exhentai.org", path="/")

    headers = {
        "User-Agent":
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
        "AppleWebKit/537.36 (KHTML, like Gecko) "
        "Chrome/141.0.0.0 Safari/537.36",
        "Referer":
        "https://e-hentai.org/",
    }

    try:
        resp = await client.get(EX_SITE + "/", headers=headers, cookies=jar)
    except Exception as e:
        print("请求 exhentai 时出错:", e)
        return None

    igneous_value = None
    for k, v in resp.cookies.items():
        if k.lower() == "igneous":
            igneous_value = v
            break

    if igneous_value and igneous_value.lower() not in ("mystery", "mysecret"):
        print("获取到 igneous:", igneous_value)
        return igneous_value
    else:
        print("未能获取有效 igneous")
        return None


@app.on_event("startup")
async def startup_event():
    base_cookies = parse_cookie_string(RAW_COOKIES)
    print("启动：检测到 cookie 键：", list(base_cookies.keys()))

    igneous = await fetch_igneous(base_cookies)

    app.state.cookies = base_cookies.copy()
    if igneous:
        app.state.cookies["igneous"] = igneous

    print("启动完成, 代理就绪")


def build_forward_cookies(base: Dict[str, str]) -> Dict[str, str]:
    return {k: v for k, v in base.items() if v}


async def stream_response(resp: httpx.Response):
    async for chunk in resp.aiter_bytes():
        yield chunk


@app.api_route(
    "/{path:path}",
    methods=["GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"])
async def proxy(path: str, request: Request):

    # 路径屏蔽
    for blocked in BLOCKED_PATHS:
        if path.startswith(blocked):
            return PlainTextResponse(
                f"Access to path '{path}' is forbidden by proxy configuration.",
                status_code=403)

    # 关键参数屏蔽
    query_str = str(request.query_params).lower()
    for qkey in BLOCKED_QUERY_KEYS:
        if qkey in query_str:
            return PlainTextResponse(
                f"Access denied: query parameter '{qkey}' is not allowed.",
                status_code=403)

    # 反代hath网络图片
    if path.startswith("hath/"):
        parts = path.split("/", 2)
        if len(parts) >= 3:
            domain = parts[1]
            subpath = parts[2]
            target_url = f"https://{domain}/{subpath}"
        else:
            return PlainTextResponse("Invalid hath URL", status_code=400)

    elif path.startswith("s/"):
        # 判断是否为阅读页（s/<token>/<gid>-<page>）
        match = re.match(r"s/[0-9a-f]{8,}/\d+-\d+", path)
        if match:
            # 主站页面
            target_url = f"https://exhentai.org/{path}"
        else:
            # 静态资源
            target_url = f"https://s.exhentai.org/{path[2:]}"

    elif path.startswith("w/"):
        # 静态图片资源
        target_url = f"https://s.exhentai.org/{path}"

    else:
        # 默认：主站内容
        target_url = EX_SITE + "/" + path if path else EX_SITE + "/"

    method = request.method
    forward_headers = {}
    for name, value in request.headers.items():
        if name.lower() in ("host", "connection", "keep-alive",
                            "proxy-authenticate", "proxy-authorization", "te",
                            "trailers", "transfer-encoding", "upgrade",
                            "cookie"):
            continue
        forward_headers[name] = value

    body = await request.body()
    cookies = build_forward_cookies(getattr(app.state, "cookies", {}))

    # 屏蔽添加新评论
    if body:
        try:
            text = body.decode("utf-8", errors="ignore")
            if "commenttext_new" in text:
                return PlainTextResponse(
                    "Blocked by proxy: comment submission is not allowed.",
                    status_code=403)
        except Exception:
            pass

    #屏蔽通过api.php的参数
    if path.endswith("api.php") and request.method.upper() == "POST":
        try:
            import json
            data = json.loads(body.decode("utf-8"))
            if data.get("method") in BLOCKED_METHODS:
                return PlainTextResponse(
                    f"Blocked by proxy: method '{data['method']}' not allowed.",
                    status_code=403)
        except Exception:
            pass

    try:
        resp = await client.request(method,
                                    target_url,
                                    headers=forward_headers,
                                    content=body if body else None,
                                    cookies=cookies,
                                    params=request.query_params,
                                    timeout=60.0)
    except httpx.RequestError as e:
        return PlainTextResponse(f"转发失败: {e}", status_code=502)

    excluded_headers = {
        "content-encoding", "transfer-encoding", "connection", "keep-alive"
    }
    headers = [(k, v) for k, v in resp.headers.items()
               if k.lower() not in excluded_headers]
    content_type = resp.headers.get("content-type", "")

    # 处理 api.php 内返回的链接
    if request.url.path.endswith("api.php"):
        import json

        proxy_base = str(request.base_url).rstrip("/")
        raw_text = resp.text

        def rewrite_hath_url(s: str) -> str:
            m = re.match(
                r"^https?://([a-z0-9.-]+\.hath\.network(?::\d+)?)(/.*)?$", s,
                re.IGNORECASE)
            if m:
                return f"{proxy_base}/hath/{m.group(1)}{m.group(2) or ''}"
            return s

        def walk_and_rewrite(obj):
            if isinstance(obj, dict):
                return {k: walk_and_rewrite(v) for k, v in obj.items()}
            if isinstance(obj, list):
                return [walk_and_rewrite(v) for v in obj]
            if isinstance(obj, str):
                return rewrite_hath_url(obj)
            return obj

        try:
            data = json.loads(raw_text)
            data = walk_and_rewrite(data)
            content = json.dumps(data, ensure_ascii=False)
        except Exception:
            content = re.sub(
                r"https?:\\\\/+([a-z0-9\.-]+\.hath\.network(?::\d+)?)(\\/[^\s\"'>]+)?",
                lambda m: f"{proxy_base}/hath/{m.group(1)}" +
                (m.group(2) or '').replace("\\/", "/"), raw_text)
            content = re.sub(
                r"https?://([a-z0-9\.-]+\.hath\.network(?::\d+)?)(/[^\s\"'>]+)?",
                lambda m: f"{proxy_base}/hath/{m.group(1)}{m.group(2) or ''}",
                content)

        clean_headers = {
            k: v
            for k, v in dict(headers).items()
            if k.lower() not in ("content-length", "content-encoding",
                                 "transfer-encoding")
        }
        # 强制声明
        clean_headers["Content-Type"] = "application/json; charset=utf-8"

        return Response(
            content=content,
            status_code=resp.status_code,
            headers=clean_headers,
        )

    if "text/html" in content_type:
        content = resp.text
        proxy_base = str(request.base_url).rstrip("/")

        for origin in [
                "https://exhentai.org", "http://exhentai.org",
                "//exhentai.org", "https://s.exhentai.org",
                "http://s.exhentai.org", "//s.exhentai.org"
        ]:
            content = content.replace(origin, proxy_base)

        # 屏蔽前端中的敏感信息（apiuid apikey）
        content = re.sub(r'var\s+apiuid\s*=\s*[^;]+;',
                         'var apiuid = "hidden";', content)
        content = re.sub(r'var\s+apikey\s*=\s*["\'][^"\']+["\'];',
                         'var apikey = "hidden";', content)

        content = re.sub(
            r"https?://([a-z0-9.-]+\.hath\.network(?::\d+)?)(/[^\s\"'>]+)?",
            lambda m: f"{proxy_base}/hath/{m.group(1)}{m.group(2) or ''}",
            content)

        clean_headers = {
            k: v
            for k, v in dict(headers).items()
            if k.lower() not in ("content-length", "content-encoding",
                                 "transfer-encoding")
        }

        await track_request(request, content)

        return Response(content=content,
                        status_code=resp.status_code,
                        headers=clean_headers,
                        media_type=content_type)

    clean_headers = {
        k: v
        for k, v in dict(headers).items()
        if k.lower() not in ("content-length", "content-encoding",
                             "transfer-encoding")
    }
    return StreamingResponse(stream_response(resp),
                             status_code=resp.status_code,
                             headers=clean_headers)
