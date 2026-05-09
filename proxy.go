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
	tagDBJSON      []byte
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
        .card { 
            background: #2a2b2e; 
            border-radius: 6px; 
            position: relative; 
            overflow: visible; 
            display: flex; 
            flex-direction: column; 
            text-decoration: none; 
            color: white; 
            z-index: 1; 
        }
        .card:hover, .card:active { 
            z-index: 100; 
        }
        .card img { 
            width: 100%; 
            aspect-ratio: 7/10; 
            object-fit: cover; 
            display: block; 
            background: #111; 
            border-radius: 6px 6px 0 0; 
        }
        .card .title { 
            padding: 8px; 
            font-size: 13px; 
            text-align: center; 
            line-height: 1.4; 
            display: -webkit-box; 
            -webkit-line-clamp: 2; 
            -webkit-box-orient: vertical; 
            overflow: hidden; 
            background: #2a2b2e; 
            border-radius: 0 0 6px 6px; 
            box-sizing: border-box; 
        }
        .card:hover .title, .card:active .title {
            position: absolute; 
            bottom: 0;
            left: 0;
            width: 100%;
            display: block;
            height: auto;
            overflow: visible;
            background: #2b2b2e;
            border-radius: 6px;
            box-shadow: 0 -4px 15px rgba(0,0,0,0.8);
            border: 1px solid #444;
        }
        .loading { text-align: center; padding: 40px; color: #888; grid-column: 1 / -1; }
        .pagination { display: flex; justify-content: center; gap: 10px; padding: 10px; }
        .pagination button { padding: 10px 20px; background: #34353b; color: white; border: none; border-radius: 4px; cursor: pointer; font-size: 15px; }
        .pagination button:disabled { opacity: 0.5; cursor: not-allowed; }
        .ehs-autocomplete-list {
            position: absolute; top: 100%; left: 0; right: 0;
            background: #2a2b2e; border: 1px solid #444; border-radius: 8px;
            z-index: 2000; max-height: 280px; overflow-y: auto;
            box-shadow: 0 4px 15px rgba(0,0,0,0.8);
            display: none; margin-top: 5px;
        }
        .ehs-autocomplete-item {
            padding: 10px 12px; border-bottom: 1px solid #333;
            display: flex; flex-direction: column; cursor: pointer;
        }
        .ehs-autocomplete-item:last-child { border-bottom: none; }
        .ehs-autocomplete-item:active { background: #3a3b3e; }
        .ehs-chs { font-size: 14px; font-weight: bold; color: #fff; }
        .ehs-eng { font-size: 12px; color: #888; margin-top: 3px; font-family: monospace; }
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
                        title = (glink.textContent || '').trim();
                    } else {
                        const textLinks = Array.from(container.querySelectorAll('a')).filter(l => (l.textContent || '').trim().length > 0);
                        if (textLinks.length > 0) {
                            title = (textLinks[0].textContent || '').trim();
                        } else {
                            title = imgNode ? (imgNode.getAttribute('title') || imgNode.getAttribute('alt')) : '';
                        }
                    }
                    
                    if (title && title !== '') {
                        seenUrls.add(href);
                        
                        let cleanHref = href;
                        const match = href.match(/\/g\/(\d+)\/([a-z0-9]+)/);
                        if (match) {
                            const gid = match[1];
                            const token = match[2];
                            
                            cleanHref = '/m-view/' + gid + '/' + token + '/';
                            
                            // ?
                            // cleanHref = '/viewer/' + gid + '/' + token + '/1/';
                        }

                        const fallbackImg = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIxMDAlIiBoZWlnaHQ9IjEwMCUiPjxyZWN0IHdpZHRoPSIxMDAlIiBoZWlnaHQ9IjEwMCUiIGZpbGw9IiMzMzMiLz48dGV4dCB4PSI1MCUiIHk9IjUwJSIgZmlsbD0iIzg4OCIgZm9udC1mYW1pbHk9InNhbnMtc2VyaWYiIGZvbnQtc2l6ZT0iMTQiIHRleHQtYW5jaG9yPSJtaWRkbGUiIGR5PSIuM2VtIj7ml6DlsIHpnaI8L3RleHQ+PC9zdmc+";
                        html += '<a class="card" href="' + cleanHref + '"><img src="' + (imgSrc || fallbackImg) + '" loading="lazy" onerror="this.src=\'' + fallbackImg + '\'"><div class="title">' + title + '</div></a>';
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

        async function initAutocomplete() {
            // 寻找输入框
            const searchInput = document.querySelector('input[name="f_search"]') || document.querySelector('input[type="text"]') || document.querySelector('input[type="search"]');
            if (!searchInput) return;

            // 禁用输入历史记录
            searchInput.setAttribute('autocomplete', 'off');

            const parent = searchInput.parentNode;
            if (window.getComputedStyle(parent).position === 'static') {
                parent.style.position = 'relative';
            }

            const acList = document.createElement('div');
            acList.className = 'ehs-autocomplete-list';
            parent.appendChild(acList);

            let tagArray = [];
            async function loadTagsForSearch() {
                try {
                    let db = null;
                    const cached = localStorage.getItem('eh_tag_db');
                    const cacheTime = localStorage.getItem('eh_tag_db_time');
                    if (cached && cacheTime && (Date.now() - parseInt(cacheTime) < 86400000)) {
                        db = JSON.parse(cached);
                    } else {
                        const res = await fetch('/proxy-api/tags');
                        db = await res.json();
                        if (Object.keys(db).length > 0) {
                            localStorage.setItem('eh_tag_db', JSON.stringify(db));
                            localStorage.setItem('eh_tag_db_time', Date.now().toString());
                        }
                    }
                    
                    if (db) {
                        for (let key in db) {
                            if (key.includes(':') && !key.startsWith('namespace:')) {
                                tagArray.push({ 
                                    eng: key, 
                                    chs: db[key].n || key
                                }); 
                            }
                        }
                    }
                } catch (e) { console.error("自动补全字典加载失败", e); }
            }

            await loadTagsForSearch();

            searchInput.addEventListener('input', function() {
                if (!tagArray.length) return;
                const val = this.value;
                const cursorPos = this.selectionStart;
                
                // 以空格切分 找到当前正在输入的词
                const textBeforeCursor = val.substring(0, cursorPos);
                const wordsBefore = textBeforeCursor.split(/\s+/);
                const currentWord = wordsBefore[wordsBefore.length - 1];

                // 没敲字时不显示
                if (currentWord.length < 1) {
                    acList.style.display = 'none';
                    return;
                }

                const lowerWord = currentWord.toLowerCase();
                
                // 模糊匹配中英文 限制最多显示 15 条
                const results = tagArray.filter(t => 
                    t.eng.includes(lowerWord) || t.chs.includes(lowerWord)
                ).slice(0, 15);

                if (results.length === 0) {
                    acList.style.display = 'none';
                    return;
                }

                acList.innerHTML = '';
                results.forEach(res => {
                    const item = document.createElement('div');
                    item.className = 'ehs-autocomplete-item';
                    item.innerHTML = '<div class="ehs-chs">' + res.chs + '</div><div class="ehs-eng">' + res.eng + '</div>';
                    
                    item.onmousedown = function(e) {
                        e.preventDefault(); 

                        const nsMap = {
                            'artist': 'a', 'character': 'c', 'female': 'f', 
                            'group': 'g', 'language': 'l', 'male': 'm', 
                            'parody': 'p', 'reclass': 'r', 'cosplayer': 'cos',
                            'mixed': 'x', 'other': 'o'
                        };
                        
                        let insertTag = res.eng;
                        if (insertTag.includes(':')) {
                            let parts = insertTag.split(':');
                            let ns = nsMap[parts[0]] || parts[0];
                            let tagValue = parts[1];
                            insertTag = tagValue.includes(' ') ? ns + ':"' + tagValue + '$"' : ns + ':' + tagValue + '$';
                        } else {
                            insertTag = insertTag.includes(' ') ? '"' + insertTag + '$"' : insertTag + '$';
                        }

                        // 截断旧词
                        const beforeWord = textBeforeCursor.substring(0, textBeforeCursor.length - currentWord.length);
                        const afterCursor = val.substring(cursorPos);
                        
                        searchInput.value = beforeWord + insertTag + ' ' + afterCursor;
                        acList.style.display = 'none';
                        
                        // 焦点恢复并把光标移到最后
                        const newPos = beforeWord.length + insertTag.length + 1;
                        searchInput.setSelectionRange(newPos, newPos);
                        searchInput.focus();
                    };
                    acList.appendChild(item);
                });
                acList.style.display = 'block';
            });
            searchInput.addEventListener('blur', () => { acList.style.display = 'none'; });
            searchInput.addEventListener('focus', function() {
                if (this.value) this.dispatchEvent(new Event('input'));
            });
        }
        setTimeout(initAutocomplete, 500);
    </script>
</body>
</html>
`

// 专属移动版画廊详情
const mobileViewHTML = `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>加载中...</title>
    <style>
        body { background: #1f2022; color: #f3f3f3; font-family: sans-serif; margin: 0; padding-bottom: 30px; }
        .header { position: sticky; top: 0; padding: 12px; background: #2a2b2e; border-bottom: 2px solid #ed2553; z-index: 100; display: flex; align-items: center; box-shadow: 0 2px 10px rgba(0,0,0,0.5); }
        .back-btn { background: none; border: none; color: #f3f3f3; font-size: 16px; font-weight: bold; cursor: pointer; padding: 0 15px 0 0; }
        .header-title { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex: 1; font-size: 15px; }

        .container { padding: 15px; }
        .loading { text-align: center; padding: 40px; color: #888; }
        
        /* 顶部信息区 */
        .info-section { display: flex; flex-direction: column; align-items: center; margin-bottom: 20px; }
        .cover { width: 100%; max-width: 320px; border-radius: 8px; box-shadow: 0 4px 15px rgba(0,0,0,0.5); margin-bottom: 15px; background:#111; }
        .title-main { font-size: 16px; font-weight: bold; text-align: center; margin-bottom: 5px; color: #fff; }
        .title-sub { font-size: 13px; text-align: center; color: #aaa; margin-bottom: 10px; }
        .meta-info { font-size: 12px; color: #888; text-align: center; background: #2a2b2e; padding: 8px; border-radius: 6px; width: 100%; box-sizing: border-box; }

        /*  阅读模式按钮组  */
        .read-group { display: flex; gap: 8px; margin-bottom: 20px; }
        .read-btn { flex: 1; background: #ed2553; color: white; text-align: center; padding: 14px 0; border-radius: 6px; font-size: 16px; font-weight: bold; text-decoration: none; box-shadow: 0 4px 10px rgba(237, 37, 83, 0.3); }
        .read-select { background: #34353b; color: white; border: 1px solid #444; border-radius: 6px; padding: 0 10px; font-size: 14px; outline: none; }

        /* 标签区 */
        .tags-section { background: #2a2b2e; border-radius: 6px; padding: 12px; margin-bottom: 20px; }
        .tag-group { display: flex; align-items: flex-start; margin-bottom: 8px; }
        .tag-cat { font-size: 12px; color: #888; width: 75px; flex-shrink: 0; padding-top: 5px; text-transform: capitalize; }
        .tag-items { display: flex; flex-wrap: wrap; gap: 6px; }
        .tag-chip { background: #34353b; color: #ddd; padding: 4px 8px; border-radius: 4px; font-size: 12px; border: 1px solid #444; }

        /* 预览图网格 */
        .thumbs-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(105px, 1fr)); gap: 8px; margin-bottom: 20px; scroll-margin-top: 65px; }
        .thumb-wrap { display: flex; align-items: center; justify-content: center; overflow: hidden; border-radius: 4px; border: 1px solid #333; background: #111; aspect-ratio: 7/10; }
        .thumb-wrap img { width: 100%; height: 100%; object-fit: cover; display: block; }
        .thumb-wrap div { zoom: 0.75; } 

        /* 翻页 */
        .pagination { display: flex; justify-content: center; overflow-x: auto; padding: 10px 0; background: #2a2b2e; border-radius: 6px; margin-bottom: 20px; }
        .pagination table { border-collapse: collapse; }
        .pagination td { padding: 0; }
        .pagination a, .pagination span { display: block; padding: 8px 12px; color: #fff; text-decoration: none; }
        .pagination .ptds { background: #ed2553; border-radius: 4px; font-weight: bold; }

        /* 评论区 */
        .comments-section h3 { font-size: 15px; margin: 0 0 10px 0; color: #ccc; border-bottom: 1px solid #333; padding-bottom: 5px; }
        .comment { background: #2a2b2e; border-radius: 6px; padding: 10px; margin-bottom: 10px; font-size: 13px; line-height: 1.5; word-break: break-all; }
        .c-head { margin-bottom: 8px; border-bottom: 1px dashed #444; padding-bottom: 6px; display: flex; align-items: center; flex-wrap: wrap; }
        .c-body a { color: #ed2553; }
        .c-body img { max-width: 100%; height: auto; }

        /* 标签详情弹窗 */
        .tag-modal { position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.7); z-index: 200; display: none; align-items: center; justify-content: center; backdrop-filter: blur(2px); }
        .tag-modal-content { background: #2a2b2e; width: 80%; max-width: 320px; border-radius: 10px; padding: 20px; box-sizing: border-box; box-shadow: 0 4px 20px rgba(0,0,0,0.8); text-align: center; }
        .tag-modal-title { font-size: 18px; font-weight: bold; color: #ed2553; margin-bottom: 5px; }
        .tag-modal-eng { font-size: 12px; color: #888; margin-bottom: 15px; font-family: monospace; }
        .tag-modal-desc { font-size: 14px; color: #ddd; line-height: 1.5; text-align: left; max-height: 50vh; overflow-y: auto; }
    </style>
</head>
<body>
    <div class="header">
        <button class="back-btn" onclick="handleBack()">❮ 返回</button>
        <div class="header-title" id="headTitle">加载中...</div>
    </div>

    <div class="container" id="content">
        <div class="loading">正在提取画廊数据, 请稍候...</div>
    </div>

    <div class="tag-modal" id="tagModal" onclick="this.style.display='none'">
        <div class="tag-modal-content" onclick="event.stopPropagation()">
            <div class="tag-modal-title" id="tmTitle">标签名</div>
            <div class="tag-modal-eng" id="tmEng">namespace:tag</div>
            <div class="tag-modal-desc" id="tmDesc">这里是标签的详细介绍...</div>
        </div>
    </div>

    <script>
        let lastLoadedUrl = '';
        let globalMpvBase = '';
        let currentMode = localStorage.getItem('preferredReadMode') || 'mpv';
        const pathParts = window.location.pathname.split('/').filter(p => p);
        const gidFromPath = pathParts[1];
        const tokenFromPath = pathParts[2];

        let tagDB = null;
        async function loadTagDB() {
            if (tagDB) return;
            try {
                const cached = localStorage.getItem('eh_tag_db');
                const cacheTime = localStorage.getItem('eh_tag_db_time');
                const now = Date.now();
                if (cached && cacheTime && (now - parseInt(cacheTime) < 86400000)) {
                    tagDB = JSON.parse(cached);
                    return;
                }
                const res = await fetch('/proxy-api/tags');
                tagDB = await res.json();
                if (Object.keys(tagDB).length > 0) {
                    localStorage.setItem('eh_tag_db', JSON.stringify(tagDB));
                    localStorage.setItem('eh_tag_db_time', now.toString());
                }
            } catch (e) {
                console.error("加载汉化标签库失败", e);
            }
        }

        function handleBack() {
            if (window.history.length > 1) {
                window.history.back();
            } else {
                window.location.replace('/mobile');
            }
        }

        function getFinalHref(origHref, pageNum) {
            if (currentMode === 'mpv') {
                return '/viewer/' + gidFromPath + '/' + tokenFromPath + '/' + pageNum + '/';
            }
            return origHref;
        }

        function showTagInfo(eng, chs, intro) {
            document.getElementById('tmTitle').innerText = chs;
            document.getElementById('tmEng').innerText = eng;
            document.getElementById('tmDesc').innerText = intro || '暂无详细介绍。';
            document.getElementById('tagModal').style.display = 'flex';
        }

        function onModeChange(newMode) {
            currentMode = newMode;
            localStorage.setItem('preferredReadMode', newMode);
            document.querySelectorAll('.thumb-wrap').forEach(a => {
                a.href = getFinalHref(a.getAttribute('data-orig'), a.getAttribute('data-page'));
            });
            const readBtn = document.querySelector('.read-btn');
            const firstThumb = document.querySelector('.thumb-wrap');
            if (readBtn && firstThumb) readBtn.href = firstThumb.href;
        }

        async function loadGallery(targetUrl, pushState = true) {
            if (!targetUrl) return;
            if (pushState) {
                const cleanPath = targetUrl.replace('/g/', '/m-view/');
                window.history.pushState({url: targetUrl}, '', cleanPath);
            }
            lastLoadedUrl = targetUrl;

            document.getElementById('content').innerHTML = '<div class="loading">正在提取画廊数据，请稍候...</div>';
            window.scrollTo(0, 0);

            try {
                const res = await fetch(targetUrl);
                const text = await res.text();
                const doc = new DOMParser().parseFromString(text, 'text/html');
                
                const gn = doc.querySelector('#gn') ? doc.querySelector('#gn').innerText : '未知标题';
                const gj = doc.querySelector('#gj') ? doc.querySelector('#gj').innerText : '';
            
                document.getElementById('headTitle').innerText = gn;
                document.title = gn;

                // 提取封面和大图
                let coverUrl = '';
                const gd1Img = doc.querySelector('#gd1 img');
                if (gd1Img) coverUrl = gd1Img.getAttribute('src');
                else {
                    const gd1Div = doc.querySelector('#gd1 div[style]');
                    if (gd1Div) {
                        const match = gd1Div.getAttribute('style').match(/url\((['"]?)(.*?)\1\)/);
                        if (match) coverUrl = match[2];
                    }
                }

                const upNode = doc.querySelector('#gdn a');
                const uploader = upNode ? upNode.innerText : '未知';
                const dateMatch = text.match(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}/);
                const posted = dateMatch ? dateMatch[0] : '未知';
                let rating = '暂无';
                const ratingNode = doc.querySelector('#rating_label');
                if (ratingNode) {
                    const rMatch = ratingNode.innerText.match(/[\d.]+/);
                    rating = rMatch ? rMatch[0] : ratingNode.innerText;
                }

                // 提取 MPV 链接并保存到全局
                const mpvNode = Array.from(doc.querySelectorAll('a')).find(a => a.getAttribute('href') && a.getAttribute('href').includes('/mpv/'));
                globalMpvBase = mpvNode ? mpvNode.getAttribute('href') : '';

                // 标签提取与自动汉化
                await loadTagDB();
                let tagsHtml = '';
                doc.querySelectorAll('#taglist tr').forEach(tr => {
                    const catEng = tr.querySelector('.tc') ? tr.querySelector('.tc').innerText.replace(':', '') : '';
                    let catChs = catEng;
                    
                    // 汉化左侧分类名
                    if (tagDB && tagDB["namespace:" + catEng]) {
                        catChs = tagDB["namespace:" + catEng].n;
                    }

                    const chips = Array.from(tr.querySelectorAll('a[href*=\"/tag/\"]')).map(a => {
                        const engTag = a.innerText;
                        let chsTag = engTag;
                        let intro = '';
                        
                        if (tagDB) {
                            const fullKey = catEng + ":" + engTag;
                            if (tagDB[fullKey]) {
                                chsTag = tagDB[fullKey].n;
                                intro = tagDB[fullKey].i;
                            } else if (tagDB[engTag]) {
                                chsTag = tagDB[engTag].n;
                                intro = tagDB[engTag].i;
                            }
                        }
                        
                        const safeIntro = intro.replace(/'/g, "\\'").replace(/"/g, "&quot;").replace(/\n/g, "<br>");
                        return '<span class=\"tag-chip\" onclick=\"showTagInfo(\'' + catEng + ':' + engTag + '\', \'' + chsTag + '\', \'' + safeIntro + '\')\">' + chsTag + '</span>';
                    }).join('');

                    if (catEng && chips) tagsHtml += '<div class=\"tag-group\"><div class=\"tag-cat\">' + catChs + '</div><div class=\"tag-items\">' + chips + '</div></div>';
                });

                // 预览图提取
                let thumbsHtml = '';
                Array.from(doc.querySelectorAll('#gdt a')).forEach((a, index) => {
                    const href = a.getAttribute('href');
                    const pageMatch = href.match(/-(\d+)$/);
                    const pageNum = pageMatch ? pageMatch[1] : '1';
                    const finalHref = getFinalHref(href, pageNum);
                    const thumbImg = a.querySelector('img');
                    const thumbDiv = a.querySelector('div[style]');
                    let inner = thumbImg ? '<img src=\"' + (thumbImg.getAttribute('src')||thumbImg.getAttribute('data-src')) + '\">' : (thumbDiv ? '<div style=\"' + thumbDiv.getAttribute('style') + '\"></div>' : '');
                    thumbsHtml += '<a class=\"thumb-wrap\" href=\"' + finalHref + '\" data-orig=\"' + href + '\" data-page=\"' + pageNum + '\">' + inner + '</a>';
                });

                // 翻页提取
                let paginationHtml = '';
                const ptb = doc.querySelector('.ptb') || doc.querySelector('.ptt');
                if (ptb) {
                    ptb.querySelectorAll('*').forEach(el => el.removeAttribute('onclick'));
                    ptb.querySelectorAll('a').forEach(a => a.setAttribute('href', '/m-view?url=' + encodeURIComponent(a.getAttribute('href'))));
                    paginationHtml = '<div class=\"pagination\">' + ptb.outerHTML + '</div>';
                }

                let commentsHtml = '';
                doc.querySelectorAll('div[id^="comment_"]').forEach((cBody, index) => {
                    const commentIdx = index + 1;
                    const wrapper = cBody.parentElement;
                    if (!wrapper) return;
                    const authorLink = wrapper.querySelector('a[href*="/uploader/"]');
                    const author = authorLink ? (authorLink.textContent || '').trim() : '未知用户';
                    let timeStr = '';
                    const c3 = wrapper.querySelector('.c3');
                    if (c3) {
                        let rawText = c3.textContent || '';
                        if (authorLink) rawText = rawText.replace(authorLink.textContent || '', '');
                        timeStr = rawText.replace(/Posted on|by:|提交于|由/gi, '').trim().replace(/(,$|^,)/g, '').trim();
                    }
                    const c7 = wrapper.querySelector('.c7');
                    let votesHtml = c7 ? c7.innerHTML : '';
                    const voteId = 'votes_' + cBody.id; 

                    let badgeHtml = '';
                    const wrapperText = wrapper.textContent || '';
                    if (/Uploader Comment|上传者/i.test(wrapperText)) {
                        badgeHtml = '<span style="color:#ed2553; border: 1px solid #ed2553; padding: 1px 4px; border-radius: 3px; font-size: 10px; margin-left: 8px;">上传者</span>';
                    } else {
                        const scoreNode = wrapper.querySelector('span[id^="comment_score_"]');
                        if (scoreNode) {
                            const actualScore = scoreNode.textContent.trim();
                            const scoreColor = actualScore.includes('+') ? '#4caf50' : '#f44336';
                            badgeHtml = '<span onclick="const v = document.getElementById(\'' + voteId + '\'); v.style.display = v.style.display === \'none\' ? \'block\' : \'none\'" style="color:' + scoreColor + '; font-weight:bold; margin-left: 8px; font-size: 12px; cursor: pointer;">[' + actualScore + ']</span>';
                        }
                    }
                    
                    let bHtml = cBody.innerHTML.replace(/href="[^"]*\/g\/(\d+)\/([a-z0-9]+)\/?[^"]*"/gi, (m, newGid, newToken) => {
                        return 'href="/m-view/' + newGid + '/' + newToken + '/"';
                    });
                    let extraVotesDiv = votesHtml ? '<div id="' + voteId + '" style="display:none; margin-top:8px; padding-top:8px; border-top:1px dashed #444; font-size:11px; color:#aaa; line-height: 1.4;">' + votesHtml + '</div>' : '';

                    commentsHtml += '<div class="comment" id="c-' + commentIdx + '">' +
                        '<div class="c-head">' +
                        '<span style="font-weight:bold; color:#ddd; font-size:13px;">' + author + '</span>' + 
                        badgeHtml + 
                        '<span style="margin-left:auto; color:#666; font-size:11px; display:flex; align-items:center;">' + 
                            '<span style="color:#888; font-size:11px; margin-right:6px;">#' + commentIdx + '</span>' + 
                            timeStr + 
                        '</span></div>' +
                        '<div class="c-body">' + bHtml + extraVotesDiv + '</div></div>';
                });

                let html = '';
                html += '<div class=\"info-section\">';
                if(coverUrl) html += '<img class=\"cover\" src=\"' + coverUrl + '\">';
                html += '<div class=\"title-main\">' + gn + '</div>';
                if(gj) html += '<div class=\"title-sub\">' + gj + '</div>';
                html += '<div class=\"meta-info\">评分: ' + rating + ' &nbsp;|&nbsp; 上传者: ' + uploader + ' &nbsp;|&nbsp; ' + posted + '</div>';
                html += '</div>';

                // 开始阅读按钮 + 下拉框
                const firstThumb = Array.from(doc.querySelectorAll('#gdt a'))[0];
                const initialBtnHref = firstThumb ? getFinalHref(firstThumb.getAttribute('href'), '1') : '#';
                
                html += '<div class=\"read-group\">' +
                            '<a class=\"read-btn\" id=\"mainReadBtn\" href=\"' + initialBtnHref + '\">▶ 开始阅读</a>' +
                            '<select class=\"read-select\" onchange=\"onModeChange(this.value)\">' +
                                '<option value=\"mpv\" ' + (currentMode === 'mpv' ? 'selected' : '') + '>多页查看器</option>' +
                                '<option value=\"normal\" ' + (currentMode === 'normal' ? 'selected' : '') + '>普通查看</option>' +
                            '</select>' +
                        '</div>';

                if (tagsHtml) html += '<div class=\"tags-section\">' + tagsHtml + '</div>';
                html += '<div class=\"thumbs-grid\" id=\"thumbsGrid\">' + thumbsHtml + '</div>';
                html += '<div id=\"paginationWrap\">' + paginationHtml + '</div>';
                if (commentsHtml) html += '<div class=\"comments-section\"><h3>评论 (' + doc.querySelectorAll('div[id^=\"comment_\"]').length + ')</h3>' + commentsHtml + '</div>';

                document.getElementById('content').innerHTML = html;

                const handleHashJump = () => {
                    const hash = window.location.hash;
                    if (hash && /^#\d+$/.test(hash)) {
                        const floor = hash.substring(1);
                        const target = document.getElementById('c-' + floor);
                        if (target) {
                            setTimeout(() => {
                                target.scrollIntoView({ behavior: 'smooth', block: 'start' });
                                target.style.boxShadow = '0 0 12px #ed2553';
                                setTimeout(() => target.style.boxShadow = 'none', 2000);
                            }, 300);
                        }
                    }
                };
                handleHashJump();
                window.onhashchange = handleHashJump;

            } catch (err) {
                document.getElementById('content').innerHTML = '<div class=\"loading\">加载失败: ' + err.message + '</div>';
            }
        }

        async function paginate(targetUrl, proxyUrl) {
            window.history.replaceState({url: targetUrl}, '', proxyUrl);
            lastLoadedUrl = targetUrl;

            const grid = document.getElementById('thumbsGrid');
            if (grid) {
                grid.innerHTML = '<div class="loading" style="grid-column: 1 / -1; padding: 30px 0;">正在拉取...</div>';
                grid.scrollIntoView({behavior: 'smooth', block: 'start'});
            }
            try {
                const res = await fetch(targetUrl);
                const text = await res.text();
                const doc = new DOMParser().parseFromString(text, 'text/html');
                let thumbsHtml = '';
                Array.from(doc.querySelectorAll('#gdt a')).forEach((a) => {
                    const href = a.getAttribute('href');
                    const pageNum = (href.match(/-(\d+)$/) || [0, '1'])[1];
                    const finalHref = getFinalHref(href, pageNum);
                    const thumbImg = a.querySelector('img');
                    const thumbDiv = a.querySelector('div[style]');
                    let inner = thumbImg ? '<img src=\"' + (thumbImg.getAttribute('src')||thumbImg.getAttribute('data-src')) + '\">' : (thumbDiv ? '<div style=\"' + thumbDiv.getAttribute('style') + '\"></div>' : '');
                    thumbsHtml += '<a class=\"thumb-wrap\" href=\"' + finalHref + '\" data-orig=\"' + href + '\" data-page=\"' + pageNum + '\">' + inner + '</a>';
                });
                let pPageHtml = '';
                const ptb = doc.querySelector('.ptb') || doc.querySelector('.ptt');
                if (ptb) {
                    ptb.querySelectorAll('*').forEach(el => el.removeAttribute('onclick'));
                    ptb.querySelectorAll('a').forEach(a => a.setAttribute('href', '/m-view?url=' + encodeURIComponent(a.getAttribute('href'))));
                    pPageHtml = '<div class=\"pagination\">' + ptb.outerHTML + '</div>';
                }
                if (grid) grid.innerHTML = thumbsHtml;
                const pWrap = document.getElementById('paginationWrap');
                if (pWrap) pWrap.innerHTML = pPageHtml;
            } catch (err) { console.error(err); }
        }

        document.addEventListener('click', function(e) {
            const link = e.target.closest('a');
            if (link && link.getAttribute('href') && link.getAttribute('href').startsWith('/m-view?url=')) {
                e.preventDefault();
                const nextTarget = new URL(link.href).searchParams.get('url');
                if (nextTarget) {
                    if (link.closest('.pagination')) paginate(nextTarget, link.href, true);
                    else loadGallery(nextTarget, true);
                }
            }
        });

        window.addEventListener('popstate', () => {
            const parts = window.location.pathname.split('/').filter(p => p);
            if (parts.length >= 3) {
                loadGallery('/g/' + parts[1] + '/' + parts[2] + '/', false);
            }
        });

        const initialUrl = '/g/' + gidFromPath + '/' + tokenFromPath + '/';
        loadGallery(initialUrl, false);
    </script>
</body>
</html>
`

// 专属移动版阅读器
const mobileViewerHTML = `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>加载中...</title>
    <style>
        body, html { margin: 0; padding: 0; width: 100%; height: 100%; background: #000; color: #fff; overflow: hidden; font-family: sans-serif; user-select: none; -webkit-user-select: none; }
        
        .ui-bar { position: absolute; left: 0; width: 100%; background: rgba(30, 30, 35, 0.95); display: flex; justify-content: space-between; align-items: center; padding: 12px 15px; box-sizing: border-box; z-index: 100; transition: transform 0.3s; backdrop-filter: blur(5px); }
        .header { top: 0; transform: translateY(-100%); border-bottom: 1px solid #444; }
        .footer { bottom: 0; transform: translateY(100%); border-top: 1px solid #444; justify-content: center; }
        .ui-bar.show { transform: translateY(0); }
        
        .btn { background: none; border: none; color: #f3f3f3; font-size: 16px; font-weight: bold; cursor: pointer; padding: 5px; }
        .page-counter { font-size: 15px; font-weight: bold; letter-spacing: 1px; }

        .viewer-container { width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; position: relative; touch-action: none; }
        .viewer-img { max-width: 100%; max-height: 100%; object-fit: contain; display: none; transform-origin: center center; will-change: transform; }
        
        .loader { width: 40px; height: 40px; border: 4px solid rgba(255,255,255,0.3); border-top: 4px solid #ed2553; border-radius: 50%; animation: spin 1s linear infinite; position: absolute; z-index: 10; box-shadow: 0 0 10px rgba(0,0,0,0.5); display: none; }
        .error-msg { position: absolute; color: #ed2553; text-align: center; padding: 20px; font-size: 14px; background: rgba(0,0,0,0.8); border-radius: 8px; display: none; z-index: 20; }
        @keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }

        .settings-modal { position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.7); z-index: 200; display: none; align-items: center; justify-content: center; backdrop-filter: blur(2px); }
        .settings-content { background: #2a2b2e; width: 85%; max-width: 400px; border-radius: 10px; padding: 20px; box-sizing: border-box; box-shadow: 0 4px 20px rgba(0,0,0,0.8); }
        .settings-content h3 { margin-top: 0; border-bottom: 1px solid #444; padding-bottom: 10px; color: #fff; }
        .setting-row { display: flex; justify-content: space-between; align-items: center; margin-bottom: 15px; }
        .setting-row select { background: #1f2022; color: #fff; border: 1px solid #555; padding: 6px 10px; border-radius: 4px; font-size: 14px; outline: none; }
        .close-settings-btn { display: block; width: 100%; background: #ed2553; color: white; border: none; padding: 12px; border-radius: 6px; font-size: 16px; font-weight: bold; margin-top: 20px; cursor: pointer; }
    </style>
</head>
<body>
    <div class="ui-bar header" id="header">
        <button class="btn" onclick="goBack()">❮ 返回</button>
        
        <div class="page-counter" id="pageMenuBtn" onclick="toggleMenu(event)" style="cursor:pointer; display:flex; align-items:center; justify-content:center; gap:5px; position:relative;">
            <span id="pageCounter">1 / -</span>
            <span id="menuArrow" style="transition: transform 0.3s ease; font-size: 10px; display:inline-block;">▼</span>
            
            <div id="viewerMenu" style="display:none; position:absolute; top:45px; left:50%; transform:translateX(-50%); background:rgba(40,40,42,0.95); border:1px solid #555; border-radius:12px; padding:5px; text-align:center; flex-direction:column; z-index: 2000; box-shadow: 0 4px 20px rgba(0,0,0,0.8); backdrop-filter: blur(5px); min-width: 130px;">
                <div onclick="downloadImage(event)" style="padding:12px; border-bottom:1px solid #444; color:#fff; font-size:14px;">下载图片</div>
                <div onclick="reloadImage(event)" style="padding:12px; border-bottom:1px solid #444; color:#fff; font-size:14px;">重载图片</div>
                <div onclick="loadOriginalImage(event)" style="padding:12px; color:#ed2553; font-weight:bold; font-size:14px;">查看原图</div>
            </div>
        </div>

        <button class="btn" onclick="openSettings()">⚙ 设置</button>
    </div>

    <div class="ui-bar footer" id="footer">
        <div style="font-size:12px; color:#aaa;" id="imgInfo">加载中...</div>
    </div>

    <div class="viewer-container" id="viewer">
        <div class="loader" id="loader"></div>
        <div class="error-msg" id="errorMsg"></div>
        <img class="viewer-img" id="mainImg" />
    </div>

    <div class="settings-modal" id="settingsModal" onclick="closeSettings(event)">
        <div class="settings-content" onclick="event.stopPropagation()">
            <h3>阅读器设置</h3>
            <div class="setting-row">
                <span>预加载页数</span>
                <select id="preloadSelect" onchange="saveSettings()">
                    <option value="1">1 页</option>
					<option value="2">2 页</option>
                    <option value="3">3 页</option>
					<option value="4">4 页</option>
                    <option value="5">5 页</option>
                </select>
            </div>
            <div class="setting-row">
                <span>点击屏幕左侧</span>
                <select id="leftTapSelect" onchange="saveSettings()">
                    <option value="prev">上一页</option>
                    <option value="next">下一页</option>
                </select>
            </div>
            <div class="setting-row">
                <span>点击屏幕右侧</span>
                <select id="rightTapSelect" onchange="saveSettings()">
                    <option value="next">下一页</option>
                    <option value="prev">上一页</option>
                </select>
            </div>
            <p style="font-size:12px; color:#888; text-align:center;">提示：点击屏幕中间 40% 区域可呼出菜单</p>
            <button class="close-settings-btn" onclick="closeSettings(event, true)">完成</button>
        </div>
    </div>

    <script>
        const pathParts = window.location.pathname.split('/').filter(p => p);
        let gid = pathParts[1] || '';
        let token = pathParts[2] || '';
        
        let mpvUrl = '/mpv/' + gid + '/' + token + '/';
        let rawUrl = '/g/' + gid + '/' + token + '/';
        let returnUrl = '/m-view/' + gid + '/' + token + '/';

        let pageFromPath = parseInt(pathParts[3]) || 0;
        let currentPage = parseInt(pathParts[3]) || 1;

        let mpvkey = '';
        let imageList = []; 
        let galleryTitle = ''; 

        let config = {
            preloadCount: parseInt(localStorage.getItem('viewer_preload')) || 3,
            leftTap: localStorage.getItem('viewer_left_tap') || 'prev',
            rightTap: localStorage.getItem('viewer_right_tap') || 'next'
        };

        function initSettings() {
            document.getElementById('preloadSelect').value = config.preloadCount;
            document.getElementById('leftTapSelect').value = config.leftTap;
            document.getElementById('rightTapSelect').value = config.rightTap;
        }

        function saveSettings() {
            config.preloadCount = parseInt(document.getElementById('preloadSelect').value);
            config.leftTap = document.getElementById('leftTapSelect').value;
            config.rightTap = document.getElementById('rightTapSelect').value;
            localStorage.setItem('viewer_preload', config.preloadCount);
            localStorage.setItem('viewer_left_tap', config.leftTap);
            localStorage.setItem('viewer_right_tap', config.rightTap);
        }

        function openSettings() {
            document.getElementById('settingsModal').style.display = 'flex';
        }

        function closeSettings(e, force = false) {
            if (force || e.target.id === 'settingsModal') {
                document.getElementById('settingsModal').style.display = 'none';
            }
        }

        function toggleUI() {
            document.getElementById('header').classList.toggle('show');
            const footer = document.getElementById('footer');
            footer.classList.toggle('show');
            
            if (!footer.classList.contains('show')) {
                document.getElementById('viewerMenu').style.display = 'none';
                document.getElementById('menuArrow').style.transform = 'rotate(0deg)';
            }
        }

        function toggleMenu(e) {
            if(e) e.stopPropagation();
            const menu = document.getElementById('viewerMenu');
            const arrow = document.getElementById('menuArrow');
            if (menu.style.display === 'none' || menu.style.display === '') {
                menu.style.display = 'flex';
                arrow.style.transform = 'rotate(180deg)';
            } else {
                menu.style.display = 'none';
                arrow.style.transform = 'rotate(0deg)';
            }
        }

        async function downloadImage() {
            toggleMenu();
            const img = document.getElementById('mainImg');
            if (!img.src) return;
            
            const infoEl = document.getElementById('imgInfo');
            const oldText = infoEl.innerText;
            infoEl.innerText = "正在打包下载...";
            
            try {
                const res = await fetch(img.src);
                const blob = await res.blob();
                const url = window.URL.createObjectURL(blob);
                const a = document.createElement('a');
                a.href = url;
                a.download = imageList[currentPage - 1].n || ('image_' + currentPage + '.jpg');
                document.body.appendChild(a);
                a.click();
                a.remove();
                window.URL.revokeObjectURL(url);
                infoEl.innerText = "下载成功";
                setTimeout(() => { infoEl.innerText = oldText; }, 2000);
            } catch (e) {
                infoEl.innerText = "下载失败";
                setTimeout(() => { infoEl.innerText = oldText; }, 2000);
            }
        }

        function reloadImage() {
            toggleMenu();
            const img = document.getElementById('mainImg');
            const loader = document.getElementById('loader');
            if (!img.src) return;
            
            loader.style.display = 'block';
            let urlObj;
            try {
                urlObj = new URL(img.src);
            } catch(e) {
                urlObj = new URL(img.src, window.location.origin);
            }
            urlObj.searchParams.set('_t', Date.now()); 
            
            const tmpImg = new Image();
            tmpImg.onload = () => {
                loader.style.display = 'none';
                img.src = urlObj.toString();
            };
            tmpImg.onerror = () => {
                loader.style.display = 'none';
                showError("重载失败，请检查网络");
            };
            tmpImg.src = urlObj.toString();
        }

        async function loadOriginalImage(e) {
            if (e && typeof e.stopPropagation === 'function') e.stopPropagation();
        
            if (typeof toggleMenu === 'function') toggleMenu();

            const img = document.getElementById('mainImg');
            const infoEl = document.getElementById('imgInfo');
            const loader = document.getElementById('loader');
            
            if (!imageList || !imageList[currentPage - 1]) return;
            const imgData = imageList[currentPage - 1];
            
            loader.style.display = 'block';
            infoEl.innerText = "正在通过 API 获取原图...";
            
            try {
                const payload = {
                    method: "imagedispatch",
                    gid: parseInt(gid),
                    page: currentPage,
                    imgkey: imgData.k,
                    mpvkey: mpvkey
                };

                const res = await fetch('/api.php', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                const data = await res.json();
                
                if (data && data.lf) {
                    const targetUrl = window.location.origin + '/' + data.lf;
                    
                    infoEl.innerText = "原图拉取中...";
                    const tmpImg = new Image();
                    tmpImg.onload = () => {
                        loader.style.display = 'none';
                        img.src = targetUrl;
                        
                        let displayInfo = "";
                        if (data.o) {
                            displayInfo = data.o.replace(/^Download original\s+/, '').replace(/\s+source$/, '').trim();
                        } else {
                            displayInfo = tmpImg.width + " x " + tmpImg.height;
                        }
                        infoEl.innerText = displayInfo + " :: 原图";
                        
                        if (typeof resetZoom === 'function') resetZoom();
                    };
                    tmpImg.onerror = () => {
                        loader.style.display = 'none';
                        infoEl.innerText = "加载原图失败，可能是配额或网络问题";
                    };
                    tmpImg.src = targetUrl;
                } else {
                    loader.style.display = 'none';
                    infoEl.innerText = "该图片未提供原图路径";
                }
            } catch (err) {
                loader.style.display = 'none';
                console.error(err);
                infoEl.innerText = "API 请求失败";
            }
        }

        function goBack() {
            if (window.history.length > 1) {
                window.history.back();
            } else if (returnUrl) {
                window.location.replace(returnUrl);
            }
        }

        async function fetchMpvData() {
            if (!gid || !token) {
                showError("缺少 URL 参数! ");
                return;
            }

            try {
                const res = await fetch(mpvUrl);
                const text = await res.text();

                const titleMatch = text.match(/<title>(.*?)<\/title>/i);
                if (titleMatch) {
                    galleryTitle = titleMatch[1].replace(/ - ExHentai\.org$/i, '').trim();
                    document.title = galleryTitle;
                }

                const gidMatch = text.match(/var gid\s*=\s*(\d+)/);
                const mpvkeyMatch = text.match(/var mpvkey\s*=\s*"([^"]+)"/);
                const imagelistMatch = text.match(/var imagelist\s*=\s*(\[.*?\]);/);

                if (!gidMatch || !mpvkeyMatch || !imagelistMatch) {
                    showError("无法从页面提取 API 密钥, 该画廊可能不支持多页查看器.");
                    return;
                }

                gid = gidMatch[1];
                mpvkey = mpvkeyMatch[1];
                imageList = JSON.parse(imagelistMatch[1]); 

                goToPage(currentPage, false);
            } catch (err) {
                showError("初始化画廊数据失败: " + err.message);
            }
        }

        async function fetchImageUrl(page) {
            if (page < 1 || page > imageList.length) return null;
            let imgData = imageList[page - 1];
            
            if (imgData.url) return imgData;

            const payload = {
                method: "imagedispatch",
                gid: parseInt(gid),
                page: page,
                imgkey: imgData.k,
                mpvkey: mpvkey
            };

            try {
                const res = await fetch('/api.php', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                const data = await res.json();
                
                if (data && data.i) {
                    imgData.url = data.i;
                    imgData.info = (data.d || "未知尺寸") + "  |  " + imgData.n;
                    return imgData;
                }
                return null;
            } catch (err) {
                return null;
            }
        }

        let loaderTimeout = null;

        async function goToPage(page) {
            if (page < 1) page = 1;
            if (page > imageList.length) page = imageList.length;

            currentPage = page;
            document.getElementById('viewerMenu').style.display = 'none';
            document.getElementById('menuArrow').style.transform = 'rotate(0deg)';
            resetZoom();
            
            window.history.replaceState({}, '', '/viewer/' + gid + '/' + token + '/' + page + '/');

            document.getElementById('pageCounter').innerText = page + " / " + imageList.length;
            
            if (galleryTitle) {
                document.title = galleryTitle + " " + page + "/" + imageList.length;
            }

            const imgEl = document.getElementById('mainImg');
            const loader = document.getElementById('loader');
            const errorEl = document.getElementById('errorMsg');
            const infoEl = document.getElementById('imgInfo');

            errorEl.style.display = 'none';
            infoEl.innerText = "加载中...";
            
            // 延迟 50ms 显示加载圈
            if (loaderTimeout) clearTimeout(loaderTimeout);
            loaderTimeout = setTimeout(() => {
                if (currentPage === page) loader.style.display = 'block';
            }, 50);

            const imgData = await fetchImageUrl(page);
            
            if (!imgData || !imgData.url) {
                clearTimeout(loaderTimeout);
                loader.style.display = 'none';
                showError("图片加载失败, 可能是配额限制或网络错误. ");
                return;
            }

            const tmpImg = new Image();
            
            // 封装一个直接上屏的函数
            const applyImage = () => {
                if (currentPage === page) {
                    clearTimeout(loaderTimeout);
                    loader.style.display = 'none';
                    imgEl.src = imgData.url;
                    imgEl.style.display = 'block';
                    infoEl.innerText = imgData.info;
                }
            };

            tmpImg.onload = applyImage;
            
            tmpImg.onerror = () => {
                if (currentPage === page) {
                    clearTimeout(loaderTimeout);
                    loader.style.display = 'none';
                    showError("图片资源获取成功, 但浏览器加载失败. ");
                }
            }
            
            tmpImg.src = imgData.url;

            // 利用 complete 属性判断是否已经存在于浏览器本地内存中
            // 如果已经在内存中, 立刻强行渲染, 不再等 onload 回调
            if (tmpImg.complete) {
                applyImage();
            }

            triggerPreload();
        }

        async function triggerPreload() {
            for (let i = 1; i <= config.preloadCount; i++) {
                let targetPage = currentPage + i;
                if (targetPage <= imageList.length) {
                    fetchImageUrl(targetPage).then(data => {
                        if (data && data.url) {
                            new Image().src = data.url;
                        }
                    });
                }
            }
        }

        function showError(msg) {
            document.getElementById('loader').style.display = 'none';
            const err = document.getElementById('errorMsg');
            err.innerText = msg;
            err.style.display = 'block';
        }

        function handleTap(e) {
            if (isDragging) return;

            const width = window.innerWidth;
            const x = e.clientX;
            
            if (x < width * 0.3) {
                executeAction(config.leftTap);
            } else if (x > width * 0.7) {
                executeAction(config.rightTap);
            } else {
                toggleUI();
            }
        }

        function executeAction(action) {
            if (action === 'prev' && currentPage > 1) {
                goToPage(currentPage - 1);
            } else if (action === 'next' && currentPage < imageList.length) {
                goToPage(currentPage + 1);
            } else if (action === 'prev' && currentPage === 1) {
                document.getElementById('header').classList.add('show');
            }
        }

        let scale = 1;
        let pointX = 0;
        let pointY = 0;
        let isDragging = false; 
        
        let initialPinchDistance = 0;
        let initialScale = 1;
        let lastTouchX = 0;
        let lastTouchY = 0;

        let touchStartTime = 0;
        let touchStartX = 0;
        let touchStartY = 0;
        let hasMoved = false; 
        let lastTapTime = 0;
        let tapTimeout = null;
        
        const viewer = document.getElementById('viewer');
        const img = document.getElementById('mainImg');

        function setTransform() {
            img.style.transform = 'translate(' + pointX + 'px, ' + pointY + 'px) scale(' + scale + ')';
        }

        function resetZoom() {
            scale = 1; pointX = 0; pointY = 0;
            img.style.transition = 'transform 0.2s ease'; 
            setTransform();
            setTimeout(() => { img.style.transition = 'none'; }, 200);
        }

        function getDistance(touches) {
            return Math.hypot(touches[0].clientX - touches[1].clientX, touches[0].clientY - touches[1].clientY);
        }

        function getMidpoint(touches) {
            return {
                x: (touches[0].clientX + touches[1].clientX) / 2,
                y: (touches[0].clientY + touches[1].clientY) / 2
            };
        }

        viewer.addEventListener('touchstart', (e) => {
            if (e.touches.length === 1) {
                touchStartTime = Date.now();
                touchStartX = e.touches[0].clientX;
                touchStartY = e.touches[0].clientY;
                hasMoved = false;
                
                if (scale > 1) {
                    lastTouchX = e.touches[0].clientX;
                    lastTouchY = e.touches[0].clientY;
                    isDragging = false;
                }
            } else if (e.touches.length === 2) {
                e.preventDefault(); // 双指操作时阻止默认事件
                initialPinchDistance = getDistance(e.touches);
                initialScale = scale;
                const mid = getMidpoint(e.touches);
                lastTouchX = mid.x;
                lastTouchY = mid.y;
                isDragging = true; 
                hasMoved = true; 
            }
        }, { passive: false });

        viewer.addEventListener('touchmove', (e) => {
            if (e.touches.length === 1) {
                // 滑动容差
                if (Math.abs(e.touches[0].clientX - touchStartX) > 10 || Math.abs(e.touches[0].clientY - touchStartY) > 10) {
                    hasMoved = true;
                }
                
                if (scale > 1) {
                    e.preventDefault(); 
                    isDragging = true;
                    pointX += e.touches[0].clientX - lastTouchX;
                    pointY += e.touches[0].clientY - lastTouchY;
                    lastTouchX = e.touches[0].clientX;
                    lastTouchY = e.touches[0].clientY;
                    setTransform();
                }
            } else if (e.touches.length === 2) {
                e.preventDefault();
                isDragging = true;
                hasMoved = true;
                
                const currentDistance = getDistance(e.touches);
                const newScale = Math.min(Math.max(0.2, initialScale * (currentDistance / initialPinchDistance)), 5);
                const scaleRatio = newScale / scale;
                const mid = getMidpoint(e.touches);
                
                const centerX = window.innerWidth / 2;
                const centerY = window.innerHeight / 2;
                
                pointX -= (mid.x - centerX - pointX) * (scaleRatio - 1);
                pointY -= (mid.y - centerY - pointY) * (scaleRatio - 1);
                
                pointX += (mid.x - lastTouchX);
                pointY += (mid.y - lastTouchY);
                
                scale = newScale;
                lastTouchX = mid.x;
                lastTouchY = mid.y;
                setTransform();
            }
        }, { passive: false });

        viewer.addEventListener('touchend', (e) => {
            if (e.touches.length === 0) {
                if (scale < 1) {
                    resetZoom();
                } else if (!hasMoved && Date.now() - touchStartTime < 300) {
                    // 触发一次轻触
                    e.preventDefault(); 
                    let currentTime = Date.now();
                    
                    // 双击判定窗口 250 毫秒内连续敲击两下
                    if (currentTime - lastTapTime < 250) {
                        clearTimeout(tapTimeout);
                        
                        if (scale > 1) {
                            // 双击恢复默认
                            resetZoom();
                        } else {
                            // 双击放大到 2.5 倍并对准点击处
                            scale = 2.5;
                            const centerX = window.innerWidth / 2;
                            const centerY = window.innerHeight / 2;
                            
                            // 使双击的位置移动到屏幕中心
                            pointX = (centerX - touchStartX) * (scale - 1);
                            pointY = (centerY - touchStartY) * (scale - 1);
                            
                            img.style.transition = 'transform 0.2s ease';
                            setTransform();
                            setTimeout(() => { img.style.transition = 'none'; }, 200);
                        }
                        
                        lastTapTime = 0; 
                    } else {
                        lastTapTime = currentTime;
                        // 延迟 200ms 执行翻页 等待判断是否会发生双击
                        tapTimeout = setTimeout(() => {
                            handleTap({ clientX: touchStartX });
                        }, 200);
                    }
                }
                setTimeout(() => { isDragging = false; }, 50); 
            } else if (e.touches.length === 1 && scale > 1) {
                lastTouchX = e.touches[0].clientX;
                lastTouchY = e.touches[0].clientY;
            }
        });

        function handleTap(e) {
            const width = window.innerWidth;
            const x = e.clientX; 
            
            if (x < width * 0.3) {
                executeAction(config.leftTap);
            } else if (x > width * 0.7) {
                executeAction(config.rightTap);
            } else {
                toggleUI();
            }
        }

        initSettings();
        fetchMpvData();
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

// 翻译标签用
type TagItem struct {
	Name  string `json:"n"`
	Intro string `json:"i"`
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

func InitTagDB() {
	fmt.Println("正在拉取 EhTagTranslation 标签数据库...")
	resp, err := http.Get("https://cdn.jsdelivr.net/gh/EhTagTranslation/DatabaseReleases/db.text.json")
	if err != nil {
		fmt.Println("获取标签数据库失败:", err)
		return
	}
	defer resp.Body.Close()

	var db struct {
		Data []struct {
			Namespace string `json:"namespace"`
			Data      map[string]struct {
				Name  string `json:"name"`
				Intro string `json:"intro"`
			} `json:"data"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&db); err != nil {
		fmt.Println("解析标签数据库失败:", err)
		return
	}

	tagMap := make(map[string]TagItem)
	for _, ns := range db.Data {
		for eng, tagData := range ns.Data {
			item := TagItem{Name: tagData.Name, Intro: tagData.Intro}
			// 保存 命名空间:标签
			tagMap[ns.Namespace+":"+eng] = item
			// 保存裸标签做后备
			if _, exists := tagMap[eng]; !exists {
				tagMap[eng] = item
			}
		}
	}

	// 补充左侧命名空间分类的汉化
	nsMap := map[string]string{
		"artist": "画师", "character": "角色", "female": "女性", "male": "男性",
		"parody": "原作", "group": "团队", "mixed": "混合", "language": "语言",
		"reclass": "重新分类", "other": "其他", "cosplayer": "Coser",
	}
	for k, v := range nsMap {
		tagMap["namespace:"+k] = TagItem{Name: v, Intro: ""}
	}

	tagDBJSON, _ = json.Marshal(tagMap)
	fmt.Printf("标签数据库加载完成，共汉化 %d 条标签\n", len(tagMap))
}

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

		total, history := GetUserStats(clientIP, fp)

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
		if tagDBJSON == nil {
			w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(tagDBJSON)
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
