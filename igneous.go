package main

import (
	"log"
	"net/http"
	"strings"
	"time"
)

func fetchIgneous(cookies map[string]string) string {
	client := &http.Client{Timeout: 30 * time.Second}
	req, _ := http.NewRequest("GET", ExSite+"/", nil)

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://e-hentai.org/")

	for k, v := range cookies {
		if strings.ToLower(k) == "igneous" {
			continue
		}
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Println("请求 exhentai 时出错:", err)
		return ""
	}
	defer resp.Body.Close()

	var igneousValue string
	for _, cookie := range resp.Cookies() {
		if strings.ToLower(cookie.Name) == "igneous" {
			igneousValue = cookie.Value
			break
		}
	}

	lowerIgneous := strings.ToLower(igneousValue)
	if igneousValue != "" && lowerIgneous != "mystery" && lowerIgneous != "mysecret" {
		log.Println("获取到 igneous:", igneousValue)
		return igneousValue
	}

	log.Println("未能获取有效 igneous")
	return ""
}
