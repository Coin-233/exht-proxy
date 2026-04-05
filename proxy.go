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

	headRegex      = regexp.MustCompile(`(?i)(<head[^>]*>)`)
	cfBeaconRegex  = regexp.MustCompile(`(?is)<script[^>]*cloudflareinsights\.com[^>]*>.*?</script>`)
	cfCommentRegex = regexp.MustCompile(`(?is)`)

	// 汉化字典
	translations   map[string]string
	jsTranslations map[string]string
)

// 手机视图
const mobileAppHTML = `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>ExHentai Mobile</title>
    <style>
        body { background: #1f2022; color: #f3f3f3; font-family: sans-serif; margin: 0; padding-bottom: 20px; }
        .header { position: sticky; top: 0; padding: 10px; background: #2a2b2e; display: flex; gap: 8px; z-index: 100; box-shadow: 0 2px 10px rgba(0,0,0,0.5); }
        .header input { flex: 1; min-width: 0; padding: 10px; border-radius: 4px; border: none; background: #444; color: white; font-size: 15px; }
        .header button { padding: 0 15px; background: #ed2553; color: white; border: none; border-radius: 4px; font-weight: bold; cursor: pointer; }
        .opt-btn { background: #444 !important; }
        
        .options-panel { display: none; padding: 12px; background: #2a2b2e; border-bottom: 2px solid #ed2553; }
        .options-panel.open { display: block; }
        .cats { display: flex; flex-wrap: wrap; gap: 8px; justify-content: center; }
        .cat-chip { padding: 6px 12px; border-radius: 15px; background: #111; color: #888; font-size: 13px; cursor: pointer; user-select: none; border: 1px solid #333; transition: all 0.2s; }
        .cat-chip.active { background: #ed2553; color: white; border-color: #ed2553; }

        #resultCount { display: none; text-align: center; font-size: 13px; color: #aaa; padding: 10px; background: #1f2022; border-bottom: 1px solid #333; }

        .grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 10px; padding: 10px; }
        .card { background: #2a2b2e; border-radius: 6px; overflow: hidden; display: flex; flex-direction: column; text-decoration: none; color: white; }
        .card img { width: 100%; aspect-ratio: 7/10; object-fit: cover; display: block; background: #111; }
        .card .title { padding: 8px; font-size: 13px; text-align: center; line-height: 1.4; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
        
        .loading { text-align: center; padding: 40px; color: #888; grid-column: 1 / -1; }
        .pagination { display: flex; justify-content: center; gap: 10px; padding: 10px; }
        .pagination button { padding: 10px 20px; background: #34353b; color: white; border: none; border-radius: 4px; cursor: pointer; font-size: 15px; }
        .pagination button:disabled { opacity: 0.5; cursor: not-allowed; }
    </style>
</head>
<body>
    <div class="header">
        <input type="text" id="searchInput" placeholder="搜索画廊..." onkeypress="if(event.key === 'Enter') doSearch()">
        <button onclick="doSearch()">搜索</button>
        <button class="opt-btn" onclick="toggleOpts()">选项</button>
    </div>

    <div class="options-panel" id="optionsPanel">
        <div class="cats" id="catContainer"></div>
    </div>
    
    <div id="resultCount"></div>
    
    <div class="grid" id="galleryGrid">
        <div class="loading">正在加载数据...</div>
    </div>

    <div class="pagination" id="pagination" style="display: none;">
        <button id="prevBtn" onclick="loadPage(prevUrl)">上一页</button>
        <button id="nextBtn" onclick="loadPage(nextUrl)">下一页</button>
    </div>

    <script>
        let prevUrl = '';
        let nextUrl = '';

        const categories = [
            { name: '同人志', val: 2 }, { name: '漫画', val: 4 }, { name: '画师 CG', val: 8 },
            { name: '游戏 CG', val: 16 }, { name: '欧美', val: 512 }, { name: '非 H', val: 256 },
            { name: '图集', val: 32 }, { name: 'Cosplay', val: 64 }, { name: '亚洲色情', val: 128 },
            { name: '杂项', val: 1 }
        ];

        const urlParams = new URLSearchParams(window.location.search);
        let currentCats = urlParams.has('f_cats') ? parseInt(urlParams.get('f_cats')) : 767; 
        
        if (urlParams.has('f_search')) {
            document.getElementById('searchInput').value = urlParams.get('f_search');
        }

        const catContainer = document.getElementById('catContainer');
        categories.forEach(c => {
            const el = document.createElement('div');
            if ((currentCats & c.val) === 0) el.classList.add('active');
            el.className = 'cat-chip ' + (el.classList.contains('active') ? 'active' : '');
            el.dataset.val = c.val;
            el.innerText = c.name;
            el.onclick = () => el.classList.toggle('active');
            catContainer.appendChild(el);
        });

        function toggleOpts() {
            document.getElementById('optionsPanel').classList.toggle('open');
        }

        async function loadPage(queryStr, pushState = true) {
            if (!queryStr) return;
            const grid = document.getElementById('galleryGrid');
            const pagination = document.getElementById('pagination');
            const resCount = document.getElementById('resultCount');
            
            grid.innerHTML = '<div class="loading">解析中，请稍候...</div>';
            pagination.style.display = 'none';
            resCount.style.display = 'none';
            window.scrollTo(0, 0);

            if (pushState) {
                window.history.pushState({}, '', '/mobile' + queryStr);
            }

            try {
                const res = await fetch('/' + queryStr);
                const text = await res.text();
                const doc = new DOMParser().parseFromString(text, 'text/html');
                
                // 搜索结果统计
                const pTags = Array.from(doc.querySelectorAll('p, div'));
                for (const p of pTags) {
                    const t = p.innerText;
                    if ((t.includes('找到') || t.includes('Found about') || t.includes('Showing')) && (t.includes('结果') || t.includes('results'))) {
                        const nums = t.match(/[\d,]+/g);
                        if (nums && nums.length > 0) {
                            resCount.innerText = '找到约 ' + nums[0] + ' 个结果';
                            resCount.style.display = 'block';
                            break;
                        }
                    }
                }

                //暴力找画廊
                let html = '';
                const seenUrls = new Set();
                
                doc.querySelectorAll('a[href*="/g/"]').forEach(a => {
                    const href = a.getAttribute('href');
                    if(seenUrls.has(href)) return; 
                    
                    const container = a.closest('tr') || a.closest('td') || a.closest('div.gld') || a.parentElement;
                    if(!container) return;
                    
                    const imgNode = container.querySelector('img');
                    const imgSrc = imgNode ? (imgNode.getAttribute('data-src') || imgNode.getAttribute('src')) : null;
                    
                    const glink = container.querySelector('.glink');
                    let title = '';
                    if (glink) {
                        title = glink.innerText.trim();
                    } else {
                        const textLinks = Array.from(container.querySelectorAll('a')).filter(l => l.innerText.trim().length > 0);
                        if (textLinks.length > 0) {
                            title = textLinks[0].innerText.trim();
                        } else {
                            title = imgNode ? (imgNode.getAttribute('title') || imgNode.getAttribute('alt')) : '';
                        }
                    }
                    
                    if (title && title !== '') {
                        seenUrls.add(href);
                        const fallbackImg = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIxMDAlIiBoZWlnaHQ9IjEwMCUiPjxyZWN0IHdpZHRoPSIxMDAlIiBoZWlnaHQ9IjEwMCUiIGZpbGw9IiMzMzMiLz48dGV4dCB4PSI1MCUiIHk9IjUwJSIgZmlsbD0iIzg4OCIgZm9udC1mYW1pbHk9InNhbnMtc2VyaWYiIGZvbnQtc2l6ZT0iMTQiIHRleHQtYW5jaG9yPSJtaWRkbGUiIGR5PSIuM2VtIj7ml6DlsIHpnaI8L3RleHQ+PC9zdmc+";
                        html += '<a class="card" href="' + href + '"><img src="' + (imgSrc || fallbackImg) + '" loading="lazy" onerror="this.src=\'' + fallbackImg + '\'"><div class="title">' + title + '</div></a>';
                    }
                });

                if(html === '') {
                    grid.innerHTML = '<div class="loading">未找到画廊。<br><br>请确保您的账号在电脑版设置中开启了【扩展视图】或【缩略图】模式</div>';
                    return;
                }

                grid.innerHTML = html;

                const pagerLinks = Array.from(doc.querySelectorAll('table.ptt a, table.ptb a, .searchnav a'));
                if (pagerLinks.length > 0) {
                    const prevNode = pagerLinks.find(a => a.innerText.includes('<') || a.innerText.includes('前') || a.innerText.includes('Prev'));
                    const nextNode = pagerLinks.find(a => a.innerText.includes('>') || a.innerText.includes('后') || a.innerText.includes('Next'));
                    
                    prevUrl = prevNode ? new URL(prevNode.getAttribute('href'), window.location.origin).search : '';
                    nextUrl = nextNode ? new URL(nextNode.getAttribute('href'), window.location.origin).search : '';
                    
                    document.getElementById('prevBtn').disabled = !prevUrl;
                    document.getElementById('nextBtn').disabled = !nextUrl;
                    pagination.style.display = 'flex';
                }

            } catch (err) {
                grid.innerHTML = '<div class="loading">加载失败, 请检查网络.</div>';
            }
        }

        function doSearch() {
            const keyword = document.getElementById('searchInput').value;
            let cats = 0;
            
            document.querySelectorAll('.cat-chip:not(.active)').forEach(chip => {
                cats += parseInt(chip.dataset.val);
            });

            let queryStr = '?';
            if (keyword) queryStr += 'f_search=' + encodeURIComponent(keyword) + '&';
            if (cats > 0) queryStr += 'f_cats=' + cats;
            
            queryStr = queryStr.replace(/[?&]$/, ''); 

            document.getElementById('optionsPanel').classList.remove('open');
            loadPage(queryStr);
        }

        window.addEventListener('popstate', () => {
            loadPage(window.location.search, false);
        });

        let initQuery = window.location.search;
        if (!initQuery) initQuery = '?f_cats=' + currentCats; 
        loadPage(initQuery, false);
    </script>
</body>
</html>
`

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

	if path == "mobile" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mobileAppHTML))
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

		isMobile := regexp.MustCompile(`(?i)(android|iphone|ipad|mobile)`).MatchString(r.UserAgent())
		// 如果是移动设备 且访问的是主页或搜索页 注入横幅
		if isMobile && (path == "" || strings.HasPrefix(path, "?")) {
			banner := `<div style="position:fixed;top:0;left:0;width:100%;background:#ed2553;text-align:center;padding:12px;z-index:999999;box-shadow:0 2px 10px rgba(0,0,0,0.5);">
				<a href="/mobile" style="color:white;text-decoration:none;font-size:16px;font-weight:bold;display:block;">检测到手机端, 点击进入专属 UI</a>
			</div>`
			bodyRegex := regexp.MustCompile(`(?i)(<body[^>]*>)`)
			content = bodyRegex.ReplaceAllString(content, "${1}\n"+banner)
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
