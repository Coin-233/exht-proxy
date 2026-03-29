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
	translations   map[string]string
	jsTranslations map[string]string
)

// 定义用于解析 JSON 的结构体
type TranslationsConfig struct {
	HTML map[string]string `json:"html"`
	JS   map[string]string `json:"js"`
}

type ProxyHandler struct {
	client  *http.Client
	cookies map[string]string
}

// 前端注入
const injectedUI = `
<style>
  #proxy-modal { display:none; position:fixed; z-index:99999; left:0; top:0; width:100%; height:100%; background:rgba(0,0,0,0.6); backdrop-filter: blur(3px); }
  #proxy-modal-content { background:#34353b; margin:8% auto; padding:25px; width:90%; max-width:650px; color:#e0e0e0; border-radius:10px; box-shadow: 0 4px 15px rgba(0,0,0,0.5);}
  .proxy-close { float:right; cursor:pointer; font-size:28px; font-weight:bold; color: #888; line-height: 20px;}
  .proxy-close:hover { color: #fff; }
  .proxy-hist-item { padding: 8px 0; border-bottom: 1px solid #444; display: flex; justify-content: space-between;}
  .proxy-hist-item a { color: #8caddf; text-decoration: none; word-break: break-all; margin-right: 15px;}
  .proxy-hist-item a:hover { text-decoration: underline; }
  /* 修改这里：给按钮加上 class 方便事件委托，如果原本就有 id 也可以保留 */
  #proxy-stats-btn, .proxy-stats-btn { cursor: pointer; color: #8caddf; font-weight: bold; }
</style>

<div id="proxy-modal">
  <div id="proxy-modal-content">
    <span class="proxy-close" onclick="document.getElementById('proxy-modal').style.display='none'">&times;</span>
    <h2 style="margin-top:0; border-bottom: 1px solid #555; padding-bottom: 10px;">您的浏览统计</h2>
    <p style="font-size: 16px;">累计访问画廊: <b id="proxy-total" style="color:#fff; font-size:18px;">0</b> 次</p>
    <h3 style="margin-bottom: 10px;">最近浏览记录 (Top 50)</h3>
    <div id="proxy-history" style="max-height:400px; overflow-y:auto; padding-right: 10px;"></div>
  </div>
</div>

<script>
  (async function(){
      const data = navigator.userAgent + screen.width + "x" + screen.height + navigator.hardwareConcurrency + navigator.language;
      const buffer = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(data));
      const fp = Array.from(new Uint8Array(buffer)).map(b => b.toString(16).padStart(2, '0')).join('').slice(0, 16);
      document.cookie = "_proxy_fp=" + fp + "; path=/; max-age=31536000";

      const escapeHTML = (str) => {
          return str.replace(/[&<>'"]/g, 
              tag => ({
                  '&': '&amp;',
                  '<': '&lt;',
                  '>': '&gt;',
                  "'": '&#39;',
                  '"': '&quot;'
              }[tag])
          );
      };

      document.addEventListener('click', function(e) {
          const btn = e.target.closest('#proxy-stats-btn');
          if (!btn) return;
          
          e.preventDefault();
          document.getElementById('proxy-modal').style.display = 'block';
          document.getElementById('proxy-history').innerHTML = "加载中...";
          document.getElementById('proxy-total').innerText = "...";
          
          fetch('/proxy-api/stats')
              .then(r => {
                  if (!r.ok) throw new Error('网络响应异常');
                  return r.json();
              })
              .then(data => {
                  document.getElementById('proxy-total').innerText = data.total || 0;
                  const histDiv = document.getElementById('proxy-history');
                  
                  if(data.history && data.history.length > 0) {
                      histDiv.innerHTML = data.history.map(item => 
                          '<div class="proxy-hist-item">' + 
                              '<a href="/' + escapeHTML(item.url) + '" target="_blank">' + escapeHTML(item.title) + '</a> ' + 
                              '<span style="font-size:12px;color:#888;min-width:130px;text-align:right;">' + escapeHTML(item.time) + '</span>' + 
                          '</div>'
                      ).join('');
                  } else {
                      histDiv.innerHTML = "<p style='color:#aaa;'>暂无浏览记录</p>";
                  }
              })
              .catch(err => {
                  console.error("Stats Fetch Error:", err);
                  document.getElementById('proxy-history').innerHTML = "<p style='color:#ff6b6b;'>加载失败，请稍后重试。</p>";
              });
      });
  })();
</script>

<div id="proxy-modal">
  <div id="proxy-modal-content">
    <span class="proxy-close" onclick="document.getElementById('proxy-modal').style.display='none'">&times;</span>
    <h2 style="margin:0 0 15px 0; border-bottom: 1px solid #444; padding-bottom: 10px; font-weight: 500;">您的浏览统计</h2>
    <p style="font-size: 15px; color: #bbb;">累计访问画廊: <b id="proxy-total" style="color:#6ab0ff; font-size:20px; margin-left:5px;">0</b> 次</p>
    <h3 style="margin: 20px 0 10px 0; font-size: 16px; color: #ddd;">最近浏览记录 (Top 50)</h3>
    <div id="proxy-history"></div>
  </div>
</div>

<script>
  (async function() {
    // 防止 XSS 攻击
    const escape = (str) => {
      if (!str) return "";
      return String(str).replace(/[&<>"']/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]));
    };

    // 生成浏览器指纹并存入 Cookie
    const getFingerprint = async () => {
      const data = navigator.userAgent + screen.width + "x" + screen.height + (navigator.hardwareConcurrency || 4) + navigator.language;
      const buffer = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(data));
      return Array.from(new Uint8Array(buffer)).map(b => b.toString(16).padStart(2, '0')).join('').slice(0, 16);
    };

    const fp = await getFingerprint();
    document.cookie = "_proxy_fp=" + fp + "; path=/; max-age=31536000; SameSite=Lax";

    document.addEventListener('click', async (e) => {
      if (e.target && (e.target.id === 'proxy-stats-btn' || e.target.closest('#proxy-stats-btn'))) {
        e.preventDefault();
        
        const modal = document.getElementById('proxy-modal');
        const histDiv = document.getElementById('proxy-history');
        const totalSpan = document.getElementById('proxy-total');

        modal.style.display = 'block';
        histDiv.innerHTML = '<p style="color:#888;">正在加载云端数据...</p>';

        try {
          const response = await fetch('/proxy-api/stats');
          if (!response.ok) throw new Error('Network response was not ok');
          
          const data = await response.json();
          totalSpan.innerText = data.total || 0;

          if (data.history && data.history.length > 0) {
		  	histDiv.innerHTML = data.history.map(item => 
		  		'<div class="proxy-hist-item">' +
                '<a href="/' + escape(item.url) + '" target="_blank">' + escape(item.title || '无标题') + '</a>' +
                '<span style="font-size:12px; color:#666; min-width:130px; text-align:right;">' + escape(item.time) + '</span>' +
            '</div>'
        ).join('');
    } else {
        histDiv.innerHTML = '<p style="color:#666; text-align:center; margin-top:20px;">暂无记录</p>';
    }
        } catch (err) {
          console.error("Fetch error:", err);
          histDiv.innerHTML = '<p style="color:#ff6b6b;">加载失败, 请稍后重试</p>';
        }
      }
    });

    // 点击遮罩层关闭模态框
    document.getElementById('proxy-modal').onclick = function(e) {
      if (e.target === this) this.style.display = 'none';
    };
  })();
</script>
`

func NewProxyHandler(cookies map[string]string) *ProxyHandler {
	return &ProxyHandler{
		client:  &http.Client{Timeout: 60 * time.Second},
		cookies: cookies,
	}
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

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

		total, history := GetUserStats(clientIP, fp)

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"total":   total,
			"history": history,
		})
		return
	}

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
		clientIP := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			clientIP = strings.Split(forwarded, ",")[0]
		}

		fp := ""
		if cookie, err := r.Cookie("_proxy_fp"); err == nil {
			fp = cookie.Value
		}

		// 记录访问历史
		if strings.HasPrefix(path, "g/") {
			matches := titleRegex.FindStringSubmatch(content)
			if len(matches) >= 2 {
				title := strings.TrimSpace(matches[1])
				go RecordVisit(clientIP, fp, path, title)
			}
		}

		// 注入带有统计按钮和弹窗
		newFooter := `<div class="dp">
			<a href="/">Front</a>
			&nbsp; 本网站为 <a href="https://exhentai.org" target="_blank">https://exhentai.org</a> 代理, 仅供预览
			&nbsp; <a id="proxy-stats-btn">浏览统计</a>
			&nbsp; <a href="https://github.com/Coin-233/exht-proxy" target="_blank">GitHub</a>
		</div>` + injectedUI

		footerRegex := regexp.MustCompile(`(?is)<div\s+class=["']dp["'][^>]*>.*?</div>`)
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
