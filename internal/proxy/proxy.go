// Package proxy handles upstream requests and serves embedded UI assets.
package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"exht-proxy/internal/config"
	"exht-proxy/internal/storage"
)

var (
	sRouteRegex  = regexp.MustCompile(`^s/[0-9a-f]{8,}/\d+-\d+`)
	hathRegex    = regexp.MustCompile(`(?i)https?://([a-z0-9.-]+\.hath\.network(?::\d+)?)(/[^\s"'>]+)?`)
	hathEscRegex = regexp.MustCompile(`(?i)https?:\\/\\/+([a-z0-9\.-]+\.hath\.network(?::\d+)?)(\\/[^\s"'>]+)?`)
	apiuidRegex  = regexp.MustCompile(`var\s+apiuid\s*=\s*[^;]+;`)
	apikeyRegex  = regexp.MustCompile(`var\s+apikey\s*=\s*["'][^"']+["'];`)
	onionRegex   = regexp.MustCompile(`(?is)<h1 class="ih">ExHentai\.org\s*-\s*<a href="[^"]*\.onion">.*?</a>\s*&nbsp;<a href="[^"]*">\[\?\]</a></h1>`)

	headRegex      = regexp.MustCompile(`(?i)(<head[^>]*>)`)
	cfBeaconRegex  = regexp.MustCompile(`(?is)<script[^>]*cloudflareinsights\.com[^>]*>.*?</script>`)
	cfCommentRegex = regexp.MustCompile(`(?is)`)

	// 配额查询
	quotaRegex   = regexp.MustCompile(`(?i)You are currently at <strong>([\d,]+)</strong> towards your account limit of <strong>([\d,]+)</strong>`)
	creditsRegex = regexp.MustCompile(`(?i)Available:\s*([\d,]+)\s*Credits`)
	kgpRegex     = regexp.MustCompile(`(?i)Available:\s*([\d,]+)\s*kGP`)

	// 预编译，避免每次请求重新编译
	mobileRegex  = regexp.MustCompile(`(?i)(android|iphone|ipad|mobile)`)
	bodyTagRegex = regexp.MustCompile(`(?i)(<body[^>]*>)`)

	hathDlRegex        = regexp.MustCompile(`(?is)<div[^>]*>\s*<p[^>]*>H@H Downloader</p>.*?</script>\s*</div>`)
	archiveCostRegex   = regexp.MustCompile(`(?is)(Download Cost:\s*(?:&nbsp;)?\s*<strong>([^<]+)</strong>.*?)<input type="submit"([^>]+)>`)
	torrentUploadRegex = regexp.MustCompile(`(?is)<div[^>]*>\s*<div[^>]*>\s*<span[^>]*>New Torrents:</span>.*?</form>\s*</div>`)
)

// 转发请求并提供 UI
type ProxyHandler struct {
	client         *http.Client
	cookies        map[string]string
	cookieMu       sync.RWMutex
	config         config.Config
	store          *storage.Store
	translations   map[string]string
	jsTranslations map[string]string
	tagDBJSON      []byte
	tagMu          sync.RWMutex
}

func NewProxyHandler(cfg config.Config, cookies map[string]string, store *storage.Store) *ProxyHandler {
	return NewProxyHandlerWithClient(cfg, cookies, store, &http.Client{Timeout: 60 * time.Second})
}

func NewProxyHandlerWithClient(cfg config.Config, cookies map[string]string, store *storage.Store, client *http.Client) *ProxyHandler {
	h := &ProxyHandler{
		client:  client,
		cookies: cookies,
		config:  cfg,
		store:   store,
	}
	h.loadTranslations()
	return h
}

func (h *ProxyHandler) UpdateCookie(key, value string) {
	h.cookieMu.Lock()
	h.cookies[key] = value
	h.cookieMu.Unlock()
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	if path == "mobile" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mobileAppHTML))
		return
	}
	if path == "m-view" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mobileViewHTML))
		return
	}
	if path == "viewer" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mobileViewerHTML))
		return
	}
	if strings.HasPrefix(path, "m-view/") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mobileViewHTML))
		return
	}
	if strings.HasPrefix(path, "viewer/") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mobileViewerHTML))
		return
	}

	// 统计用
	if path == "proxy-api/stats" {
		clientIP := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			clientIP = strings.Split(forwarded, ",")[0]
		}

		fp := ""
		if cookie, err := r.Cookie("_proxy_fp"); err == nil {
			fp = cookie.Value
		}

		total, history := h.store.GetUserStats(clientIP, fp)

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"total":   total,
			"history": history,
		})
		return
	}

	// 标签汉化
	if path == "proxy-api/tags" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		h.tagMu.RLock()
		defer h.tagMu.RUnlock()
		if h.tagDBJSON == nil {
			w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(h.tagDBJSON)
		return
	}

	// 配额查询
	if path == "proxy-api/quota" {
		fetchHTML := func(subPath string) string {
			targetURL := "https://e-hentai.org/" + subPath
			req, err := http.NewRequest("GET", targetURL, nil)
			if err != nil {
				return ""
			}
			h.cookieMu.RLock()
			for k, v := range h.cookies {
				if v != "" {
					req.AddCookie(&http.Cookie{Name: k, Value: v})
				}
			}
			h.cookieMu.RUnlock()
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
			resp, err := h.client.Do(req)
			if err != nil {
				return ""
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			return string(b)
		}
		homeHTML := fetchHTML("home.php")
		// fmt.Println(len(homeHTML))
		// fmt.Println(homeHTML[:min(10000, len(homeHTML))])
		exchangeHTML := fetchHTML("exchange.php?t=gp")

		result := map[string]string{
			"quota_used":  "0",
			"quota_limit": "0",
			"credits":     "0",
			"kgp":         "0",
		}

		if m := quotaRegex.FindStringSubmatch(homeHTML); len(m) == 3 {
			result["quota_used"] = strings.ReplaceAll(m[1], ",", "")
			result["quota_limit"] = strings.ReplaceAll(m[2], ",", "")
		}
		if m := creditsRegex.FindStringSubmatch(exchangeHTML); len(m) == 2 {
			result["credits"] = strings.ReplaceAll(m[1], ",", "")
		}
		if m := kgpRegex.FindStringSubmatch(exchangeHTML); len(m) == 2 {
			result["kgp"] = strings.ReplaceAll(m[1], ",", "")
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(result)
		return
	}

	// 种子模式
	isTorrentPage := strings.HasPrefix(path, "gallerytorrents.php")

	if isTorrentPage && h.config.TorrentMode == 1 {
		http.Error(w, "Access is forbidden by proxy configuration.", http.StatusForbidden)
		return
	}

	// 归档模式
	isArchiverPage := strings.HasPrefix(path, "archiver.php")

	if isArchiverPage && h.config.ArchiverMode == 1 {
		http.Error(w, "Access is forbidden by proxy configuration.", http.StatusForbidden)
		return
	}

	// 路径屏蔽
	for _, blocked := range h.config.BlockedPaths {
		if blocked == "gallerytorrents.php" || blocked == "archiver.php" {
			continue
		}
		if strings.HasPrefix(path, blocked) {
			http.Error(w, fmt.Sprintf("Access to path '%s' is forbidden by proxy configuration.", path), http.StatusForbidden)
			return
		}
	}

	// 关键参数屏蔽
	queryStr := strings.ToLower(r.URL.RawQuery)
	for _, qkey := range h.config.BlockedQueryKeys {
		if strings.Contains(queryStr, qkey) {
			http.Error(w, fmt.Sprintf("Access denied: query parameter '%s' is not allowed.", qkey), http.StatusForbidden)
			return
		}
	}

	// 路由判断
	var targetURL string
	if strings.HasPrefix(path, "hath/") {
		parts := strings.SplitN(path, "/", 3)
		if len(parts) >= 3 {
			targetURL = fmt.Sprintf("https://%s/%s", parts[1], parts[2])
		} else {
			http.Error(w, "Invalid hath URL", http.StatusBadRequest)
			return
		}
	} else if isTorrentPage && h.config.TorrentMode == 2 {
		targetURL = "https://e-hentai.org/" + path
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}
	} else if strings.HasPrefix(path, "s/") {
		if sRouteRegex.MatchString(path) {
			targetURL = fmt.Sprintf("https://exhentai.org/%s", path)
		} else {
			targetURL = fmt.Sprintf("https://s.exhentai.org/%s", path[2:])
		}
	} else if strings.HasPrefix(path, "w/") {
		targetURL = fmt.Sprintf("https://s.exhentai.org/%s", path)
	} else {
		targetURL = h.config.ExSite + "/" + path
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}
	}

	// 读取 Body 并检测（限制 10MB 防止内存耗尽）
	var bodyBytes []byte
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		bodyBytes, _ = io.ReadAll(io.LimitReader(r.Body, 10<<20))
		r.Body.Close()

		text := string(bodyBytes)
		if strings.Contains(text, "commenttext_new") {
			http.Error(w, "Blocked by proxy: comment submission is not allowed.", http.StatusForbidden)
			return
		}

		if strings.HasPrefix(path, "archiver.php") && strings.Contains(text, "hathdl_xres=") {
			http.Error(w, "Blocked by proxy: H@H download is not allowed.", http.StatusForbidden)
			return
		}

		if isArchiverPage && h.config.ArchiverMode == 2 && r.Method == http.MethodPost {
			if strings.Contains(text, "hathdl_xres=") {
				http.Error(w, "Blocked by proxy: H@H downloads are not allowed.", http.StatusForbidden)
				return
			}

			dltype := ""
			if strings.Contains(text, "dltype=org") {
				dltype = "org"
			} else if strings.Contains(text, "dltype=res") {
				dltype = "res"
			}

			if dltype != "" {
				verifyReq, _ := http.NewRequest("GET", targetURL, nil)
				h.cookieMu.RLock()
				for k, v := range h.cookies {
					if v != "" {
						verifyReq.AddCookie(&http.Cookie{Name: k, Value: v})
					}
				}
				h.cookieMu.RUnlock()

				verifyResp, err := h.client.Do(verifyReq)
				isFree := false
				if err == nil {
					defer verifyResp.Body.Close()
					bVerify, _ := io.ReadAll(verifyResp.Body)
					htmlStr := string(bVerify)

					blocks := strings.Split(htmlStr, `<div style="width:180px;`)
					for _, block := range blocks {
						if strings.Contains(block, `name="dltype"`) && strings.Contains(block, `value="`+dltype+`"`) {
							if strings.Contains(strings.ToLower(block), "free") {
								isFree = true
							}
							break
						}
					}
				}

				if !isFree {
					http.Error(w, "Blocked by proxy: non-free downloads are not allowed.", http.StatusForbidden)
					return
				}
			}
		}

		if strings.HasSuffix(path, "api.php") && r.Method == http.MethodPost {
			var data map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &data); err == nil {
				if method, ok := data["method"].(string); ok {
					for _, bm := range h.config.BlockedMethods {
						if method == bm {
							http.Error(w, fmt.Sprintf("Blocked by proxy: method '%s' not allowed.", method), http.StatusForbidden)
							return
						}
					}
				}
			}
		}
	}

	// 构建新请求
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		http.Error(w, "创建请求失败", http.StatusInternalServerError)
		return
	}

	for name, values := range r.Header {
		nameLower := strings.ToLower(name)
		if nameLower == "host" || nameLower == "connection" || nameLower == "keep-alive" ||
			nameLower == "proxy-authenticate" || nameLower == "proxy-authorization" || nameLower == "te" ||
			nameLower == "trailers" || nameLower == "transfer-encoding" || nameLower == "upgrade" ||
			nameLower == "cookie" || nameLower == "accept-encoding" {
			continue
		}
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}

	h.cookieMu.RLock()
	for k, v := range h.cookies {
		if v != "" {
			if isTorrentPage && h.config.TorrentMode == 2 {
				continue
			}
			req.AddCookie(&http.Cookie{Name: k, Value: v})
		}
	}
	h.cookieMu.RUnlock()

	if isTorrentPage && h.config.TorrentMode == 2 {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
	}

	// 执行请求
	resp, err := h.client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("转发失败: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 处理响应
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	proxyBase := fmt.Sprintf("%s://%s", scheme, r.Host)
	contentType := resp.Header.Get("Content-Type")

	// 写回安全的 Header
	for k, v := range resp.Header {
		kl := strings.ToLower(k)
		if kl == "content-length" || kl == "content-encoding" || kl == "transfer-encoding" || kl == "connection" || kl == "keep-alive" {
			continue
		}
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}

	// 封禁响应必须在 API 改写, 汉化和 UI 注入之前返回
	if strings.HasPrefix(strings.ToLower(contentType), "text/") || contentType == "" || strings.HasSuffix(path, "api.php") {
		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "读取上游响应失败", http.StatusBadGateway)
			return
		}
		if message, banned := h.ipBanMessage(string(respBytes)); banned {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(resp.StatusCode)
			w.Write([]byte(message))
			return
		}
		resp.Body = io.NopCloser(bytes.NewReader(respBytes))
	}

	if strings.HasSuffix(path, "api.php") {
		respBytes, _ := io.ReadAll(resp.Body)
		content := string(respBytes)

		// 替换转义的 hath URL
		content = hathEscRegex.ReplaceAllStringFunc(content, func(m string) string {
			matches := hathEscRegex.FindStringSubmatch(m)
			sub := strings.ReplaceAll(matches[2], "\\/", "/")
			return fmt.Sprintf("%s/hath/%s%s", proxyBase, matches[1], sub)
		})
		// 替换普通的 hath URL
		content = hathRegex.ReplaceAllStringFunc(content, func(m string) string {
			matches := hathRegex.FindStringSubmatch(m)
			return fmt.Sprintf("%s/hath/%s%s", proxyBase, matches[1], matches[2])
		})

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(resp.StatusCode)
		w.Write([]byte(content))
		return
	}

	if strings.Contains(contentType, "text/html") {
		respBytes, _ := io.ReadAll(resp.Body)
		content := string(respBytes)

		// 域名替换（单次遍历替换，避免 6 次 ReplaceAll）
		originReplacer := strings.NewReplacer(
			"https://exhentai.org", proxyBase,
			"http://exhentai.org", proxyBase,
			"//exhentai.org", proxyBase,
			"https://s.exhentai.org", proxyBase,
			"http://s.exhentai.org", proxyBase,
			"//s.exhentai.org", proxyBase,
		)
		content = originReplacer.Replace(content)

		// 屏蔽前端敏感信息
		content = apiuidRegex.ReplaceAllString(content, `var apiuid = "hidden";`)
		content = apikeyRegex.ReplaceAllString(content, `var apikey = "hidden";`)

		// Hath 替换
		content = hathRegex.ReplaceAllStringFunc(content, func(m string) string {
			matches := hathRegex.FindStringSubmatch(m)
			return fmt.Sprintf("%s/hath/%s%s", proxyBase, matches[1], matches[2])
		})

		content = onionRegex.ReplaceAllString(content, `<h1 class="ih">ExHentai.org</h1>`)

		// 界面汉化
		for eng, chs := range h.translations {
			content = strings.ReplaceAll(content, eng, chs)
		}

		if isArchiverPage && h.config.ArchiverMode == 2 {
			content = hathDlRegex.ReplaceAllString(content, "")
			content = archiveCostRegex.ReplaceAllStringFunc(content, func(m string) string {
				matches := archiveCostRegex.FindStringSubmatch(m)
				if len(matches) >= 4 {
					costStr := strings.ToLower(matches[2])
					if !strings.Contains(costStr, "free") {
						disabledBtn := `<input type="button" disabled="disabled" value="禁止扣费下载" style="width:180px; background:#444; color:#888; border:1px solid #555; cursor:not-allowed;" title="代理服务器已屏蔽消耗 GP/Credits 的下载" />`
						return matches[1] + disabledBtn
					}
				}
				return m
			})
		}

		if isTorrentPage {
			content = torrentUploadRegex.ReplaceAllString(content, "")
		}

		// 提取归档包链接
		if strings.HasPrefix(path, "hath/") {
			parts := strings.SplitN(path, "/", 3)
			if len(parts) >= 2 {
				hathHost := parts[1]
				directPrefix := fmt.Sprintf(`href="https://%s/archive/`, hathHost)
				content = strings.ReplaceAll(content, `href="/archive/`, directPrefix)
			}
		}

		isMobile := mobileRegex.MatchString(r.UserAgent())
		// 如果是移动设备 且访问的是主页或搜索页 注入横幅
		if isMobile && (path == "" || strings.HasPrefix(path, "?")) {
			banner := `<div style="position:fixed;top:0;left:0;width:100%;background:#ed2553;text-align:center;padding:12px;z-index:999999;box-shadow:0 2px 10px rgba(0,0,0,0.5);">
				<a href="/mobile" style="color:white;text-decoration:none;font-size:16px;font-weight:bold;display:block;">检测到手机端, 点击进入专属 UI</a>
			</div>`
			content = bodyTagRegex.ReplaceAllString(content, "${1}\n"+banner)
		}

		// 去除 beacon 追踪
		content = cfBeaconRegex.ReplaceAllString(content, "")
		content = cfCommentRegex.ReplaceAllString(content, "")

		// // 注入 viewpoint
		// viewportMeta := "$1\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no\">"
		// content = headRegex.ReplaceAllString(content, viewportMeta)

		// 替换页脚
		clientIP := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			clientIP = strings.Split(forwarded, ",")[0]
		}

		fp := ""
		if cookie, err := r.Cookie("_proxy_fp"); err == nil {
			fp = cookie.Value
		}

		// 记录访问历史（异步，使用 worker pool 避免 goroutine 无限增长）
		if strings.HasPrefix(path, "g/") {
			matches := titleRegex.FindStringSubmatch(content)
			if len(matches) >= 2 {
				title := strings.TrimSpace(matches[1])
				h.store.EnqueueVisit(clientIP, fp, path, title)
			}
		}

		// 注入带有统计按钮和弹窗
		newFooter := `<div class="dp">
			<a href="/">Front</a>
			&nbsp; 本网站为 <a href="https://exhentai.org" target="_blank">https://exhentai.org</a> 代理, 仅供预览
			&nbsp; <a id="proxy-stats-btn">浏览统计</a>
			&nbsp; <a href="https://github.com/Coin-233/exht-proxy" target="_blank">GitHub</a>
		</div>` + injectedUI

		if footerRegex.MatchString(content) {
			content = footerRegex.ReplaceAllString(content, newFooter)
		} else {
			content = content + "\n" + newFooter
		}

		// 记录访问日志
		go logRequest(clientIP, path, content)

		w.WriteHeader(resp.StatusCode)
		w.Write([]byte(content))
		return
	} else if strings.Contains(contentType, "javascript") || strings.HasSuffix(path, ".js") {
		// 拦截 JavaScript 文件并进行汉化替换
		respBytes, _ := io.ReadAll(resp.Body)
		content := string(respBytes)

		originReplacer := strings.NewReplacer(
			"https://exhentai.org", proxyBase,
			"http://exhentai.org", proxyBase,
			"//exhentai.org", proxyBase,
			"https://s.exhentai.org", proxyBase,
			"http://s.exhentai.org", proxyBase,
			"//s.exhentai.org", proxyBase,
		)
		content = originReplacer.Replace(content)

		// 执行 JS 字符串汉化
		for eng, chs := range h.jsTranslations {
			content = strings.ReplaceAll(content, eng, chs)
		}

		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(resp.StatusCode)
		w.Write([]byte(content))
		return
	}

	// 静态资源直接流式返回
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
