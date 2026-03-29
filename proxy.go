package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	sRouteRegex  = regexp.MustCompile(`^s/[0-9a-f]{8,}/\d+-\d+`)
	hathRegex    = regexp.MustCompile(`(?i)https?://([a-z0-9.-]+\.hath\.network(?::\d+)?)(/[^\s"'>]+)?`)
	hathEscRegex = regexp.MustCompile(`(?i)https?:\\/\\/+([a-z0-9\.-]+\.hath\.network(?::\d+)?)(\\/[^\s"'>]+)?`)
	apiuidRegex  = regexp.MustCompile(`var\s+apiuid\s*=\s*[^;]+;`)
	apikeyRegex  = regexp.MustCompile(`var\s+apikey\s*=\s*["'][^"']+["'];`)
	onionRegex   = regexp.MustCompile(`(?is)<h1 class="ih">ExHentai\.org\s*-\s*<a href="[^"]*\.onion">.*?</a>\s*&nbsp;<a href="[^"]*">\[\?\]</a></h1>`)

	// 汉化字典
	translations = map[string]string{
		`>Front<span class="nbw"> Page<`:		`>首页<`,
		`>Popular<`:							`>热门<`,
		`>Watched<`:							`><`,
		`>Torrents<`:							`><`,
		`>My Tags<`:                            `><`,
		`>My </span>Uploads<`:                  `><`,
		`>Settings<`:                           `><`,
		`>Fav<span class="nbw">orite</span>s<`: `><`,
		`>Doujinshi<`:                          `>同人志<`,
		`>Manga<`:                              `>漫画<`,
		`>Artist CG<`:                          `>画师 CG<`,
		`>Game CG<`:                            `>游戏 CG<`,
		`>Western<`:                            `>欧美<`,
		`>Non-H<`:                              `>无 H<`,
		`>Image Set<`:                          `>图集<`,
		`>Cosplay<`:                            `>Cosplay<`,
		`>Asian Porn<`:                         `>亚洲色情<`,
		`>Misc<`:                               `>杂项<`,
		`placeholder="Search Keywords"`:        `placeholder="搜索关键字"`,
		`value="Search"`:                       `value="搜索"`,
		`value="Clear"`:                        `value="清除"`,
		`Show Advanced Options`:                `显示高级选项`,
		`Show File Search`:                     `显示文件搜索`,
		`&lt;&lt; First`:                       `&lt;&lt; 首页`,
		`&lt; Prev`:                            `&lt; 前一页`,
		`Next &gt;`:                            `后一页 &gt;`,
		`Last &gt;&gt;`:                        `末页 &gt;&gt;`,
		`<p>Found about `:                      `<p>找到 `,
		`results. `:                            `个结果. `,
		`Filtered`:								`过滤了`,
		`galleries from this page`:				`个结果`,
		`>Multi-Page Viewer<`:                  `>多页查看器<`,
		`title="language:chinese">chinese<`:    `title="language:chinese">汉语<`,
	}

	// JS 动态生成的界面汉化字典
	jsTranslations = map[string]string{
		`"Hide Advanced Options"`:     `"隐藏高级选项"`,
		`"Show Advanced Options"`:     `"显示高级选项"`,
		`"Hide File Search"`:          `"隐藏文件搜索"`,
		`"Show File Search"`:          `"显示文件搜索"`,
		`Browse Expunged Galleries`:   `浏览被删除的画廊`,
		`Require Gallery Torrent`:     `要求画廊包含种子`,
		`Between <input`:              `介于 <input`,
		`/> and <input`:               `/> 到 <input`,
		`/> pages`:                    `/> 页`,
		`Minimum Rating:`:             `最低评分:`,
		`>Any Rating<`:                `>任意评分<`,
		`>2 Stars<`:                   `>2 星<`,
		`>3 Stars<`:                   `>3 星<`,
		`>4 Stars<`:                   `>4 星<`,
		`>5 Stars<`:                   `>5 星<`,
		`Disable custom filters for:`: `禁用以下自定义过滤器:`,
		`<span></span> Language`:      `<span></span> 语言`,
		`<span></span> Uploader`:      `<span></span> 上传者`,
		`<span></span> Tags`:          `<span></span> 标签`,
		`Select a file to upload, then hit File Search. All public galleries containing this exact file will be displayed.`: `选择要上传的文件，然后点击文件搜索。将显示包含此确切文件的所有公开画廊。`,
		`For color images, the system can also perform a similarity lookup to find resampled images.`:                       `对于彩色图像，系统还可以执行相似度查找以寻找重采样图像。`,
		`<span></span> Use Similarity Scan`: `<span></span> 使用相似度扫描`,
		`<span></span> Only Search Covers`:  `<span></span> 仅搜索封面`,
		`value="File Search"`:               `value="文件搜索"`,
		`>Use Date Selector<`:               `>使用日期选择器<`,
		`placeholder="date or offset"`:      `placeholder="日期或偏移量"`,
		`>Close<`:                           `>关闭<`,
		`>Cancel<`:                          `>取消<`,
		`>Jump/Seek<`:                       `>跳转/搜寻<`,
		`"&lt; Seek"`:                       `"&lt; 搜寻"`,
		`"Seek &gt;"`:                       `"搜寻 &gt;"`,
		`"&lt; Jump"`:                       `"&lt; 跳转"`,
		`"Jump &gt;"`:                       `"跳转 &gt;"`,
		`"&lt; Prev"`:                       `"&lt; 上一页"`,
		`"Next &gt;"`:                       `"下一页 &gt;"`,
	}
)

type ProxyHandler struct {
	client  *http.Client
	cookies map[string]string
}

func NewProxyHandler(cookies map[string]string) *ProxyHandler {
	return &ProxyHandler{
		client:  &http.Client{Timeout: 60 * time.Second},
		cookies: cookies,
	}
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	// 路径屏蔽
	for _, blocked := range BlockedPaths {
		if strings.HasPrefix(path, blocked) {
			http.Error(w, fmt.Sprintf("Access to path '%s' is forbidden by proxy configuration.", path), http.StatusForbidden)
			return
		}
	}

	// 关键参数屏蔽
	queryStr := strings.ToLower(r.URL.RawQuery)
	for _, qkey := range BlockedQueryKeys {
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
	} else if strings.HasPrefix(path, "s/") {
		if sRouteRegex.MatchString(path) {
			targetURL = fmt.Sprintf("https://exhentai.org/%s", path)
		} else {
			targetURL = fmt.Sprintf("https://s.exhentai.org/%s", path[2:])
		}
	} else if strings.HasPrefix(path, "w/") {
		targetURL = fmt.Sprintf("https://s.exhentai.org/%s", path)
	} else {
		targetURL = ExSite + "/" + path
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}
	}

	// 读取 Body 并检测
	var bodyBytes []byte
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		bodyBytes, _ = io.ReadAll(r.Body)
		r.Body.Close()

		text := string(bodyBytes)
		if strings.Contains(text, "commenttext_new") {
			http.Error(w, "Blocked by proxy: comment submission is not allowed.", http.StatusForbidden)
			return
		}

		if strings.HasSuffix(path, "api.php") && r.Method == http.MethodPost {
			var data map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &data); err == nil {
				if method, ok := data["method"].(string); ok {
					for _, bm := range BlockedMethods {
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
	req, err := http.NewRequest(r.Method, targetURL, bytes.NewBuffer(bodyBytes))
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

	for k, v := range h.cookies {
		if v != "" {
			req.AddCookie(&http.Cookie{Name: k, Value: v})
		}
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

		// 域名替换
		origins := []string{
			"https://exhentai.org", "http://exhentai.org", "//exhentai.org",
			"https://s.exhentai.org", "http://s.exhentai.org", "//s.exhentai.org",
		}
		for _, origin := range origins {
			content = strings.ReplaceAll(content, origin, proxyBase)
		}

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
		for eng, chs := range translations {
			content = strings.ReplaceAll(content, eng, chs)
		}

		// 替换页脚
		content = replaceFooter(content)

		// 记录访问日志
		clientIP := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			clientIP = strings.Split(forwarded, ",")[0]
		}
		go logRequest(clientIP, path, content)

		w.WriteHeader(resp.StatusCode)
		w.Write([]byte(content))
		return
	} else if strings.Contains(contentType, "javascript") || strings.HasSuffix(path, ".js") {
		// 拦截 JavaScript 文件并进行汉化替换
		respBytes, _ := io.ReadAll(resp.Body)
		content := string(respBytes)

		origins := []string{
			"https://exhentai.org", "http://exhentai.org", "//exhentai.org",
			"https://s.exhentai.org", "http://s.exhentai.org", "//s.exhentai.org",
		}
		for _, origin := range origins {
			content = strings.ReplaceAll(content, origin, proxyBase)
		}

		// 执行 JS 字符串汉化
		for eng, chs := range jsTranslations {
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
