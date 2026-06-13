package main

import (
	"log"
	"regexp"
	"strings"
	"sync"
)

// maxTitlesPerIP 限制每个 IP 在内存中保留的唯一标题数，防止长期运行内存泄漏
const maxTitlesPerIP = 500

var (
	visitCount    = make(map[string]int)
	ipSeenTitles  = make(map[string]map[string]bool)
	statsMutex    sync.Mutex
	titleRegex    = regexp.MustCompile(`(?is)<h1\s+id=["']gn["']\s*>\s*(.*?)\s*</h1>`)
	footerRegex   = regexp.MustCompile(`(?is)<div\s+class=["']dp["'][^>]*>.*?</div>`)
	NewFooterHTML = `<div class="dp"><a href="/">Front</a>&nbsp; 本网站为 <a href="https://exhentai.org" target="_blank">https://exhentai.org</a> 代理, 仅供预览 &nbsp; <a href="https://github.com/Coin-233/exht-proxy" target="_blank">GitHub</a></div>`
)

func logRequest(ip, path, html string) {
	if !strings.HasPrefix(path, "g/") {
		return
	}

	matches := titleRegex.FindStringSubmatch(html)
	if len(matches) < 2 {
		return
	}
	title := strings.TrimSpace(matches[1])

	statsMutex.Lock()
	defer statsMutex.Unlock()

	if _, ok := ipSeenTitles[ip]; !ok {
		ipSeenTitles[ip] = make(map[string]bool)
	}

	// 达到上限时重置该 IP 的计数，防止内存无限增长
	if len(ipSeenTitles[ip]) >= maxTitlesPerIP {
		ipSeenTitles[ip] = make(map[string]bool)
		visitCount[ip] = 0
	}

	if !ipSeenTitles[ip][title] {
		ipSeenTitles[ip][title] = true
		visitCount[ip]++
		log.Printf("%s: %d - %s\n", ip, visitCount[ip], title)
	}
}

func replaceFooter(html string) string {
	if footerRegex.MatchString(html) {
		return footerRegex.ReplaceAllString(html, NewFooterHTML)
	}
	return html + "\n" + NewFooterHTML
}
