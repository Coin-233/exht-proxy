package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var (
	ExSite           = "https://exhentai.org"
	RawCookies       string
	BlockedPaths     []string
	BlockedQueryKeys []string
	BlockedMethods   []string
	ShowLog          bool
)

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if ShowLog {
			log.Printf("[访问日志] %s - %s %s - 耗时: %v", r.RemoteAddr, r.Method, r.URL.Path, time.Since(start))
		}
	})
}

func loadTranslations() {
	translations = make(map[string]string)
	jsTranslations = make(map[string]string)

	file, err := os.Open("translations.txt")
	if err != nil {
		log.Printf("警告: 无法打开 translations.txt, 汉化将不会生效。错误: %v\n", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var currentTarget map[string]string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 忽略空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if line == "[HTML]" {
			currentTarget = translations
			continue
		} else if line == "[JS]" {
			currentTarget = jsTranslations
			continue
		}

		if currentTarget != nil {
			parts := strings.SplitN(line, " === ", 2)
			if len(parts) == 2 {
				currentTarget[parts[0]] = parts[1]
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("读取 translations.txt 时发生错误: %v\n", err)
	} else {
		log.Printf("成功加载汉化字典: HTML(%d条) JS(%d条)\n", len(translations), len(jsTranslations))
	}
}

func main() {
	_ = godotenv.Load()
	loadTranslations()

	ShowLog = os.Getenv("SHOW_LOG") == "1"
	if ShowLog {
		log.Println("已开启详细访问日志输出")
	}

	RawCookies = strings.Trim(os.Getenv("COOKIES"), `"' `)

	// 解析屏蔽的路径
	if paths := os.Getenv("BLOCKED_PATHS"); paths != "" {
		for _, p := range strings.Split(paths, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				BlockedPaths = append(BlockedPaths, strings.TrimLeft(trimmed, "/"))
			}
		}
	} else {
		BlockedPaths = []string{"archiver.php", "mytags", "repo/torrent_post.php", "gallerytorrents.php", "uconfig.php", "favorites.php"}
	}

	// 解析屏蔽的查询参数
	if keys := os.Getenv("BLOCKED_QUERY_KEYS"); keys != "" {
		for _, k := range strings.Split(keys, ",") {
			if trimmed := strings.TrimSpace(k); trimmed != "" {
				BlockedQueryKeys = append(BlockedQueryKeys, strings.ToLower(trimmed))
			}
		}
	} else {
		BlockedQueryKeys = []string{"report", "act", "inline_set"}
	}

	// 解析屏蔽的方法
	if methods := os.Getenv("BLOCKED_METHODS"); methods != "" {
		for _, m := range strings.Split(methods, ",") {
			if trimmed := strings.TrimSpace(m); trimmed != "" {
				BlockedMethods = append(BlockedMethods, trimmed)
			}
		}
	} else {
		BlockedMethods = []string{"rategallery", "votecomment", "favorite", "taggallery"}
	}

	// 获取 Igneous Cookie
	baseCookies := parseCookies(RawCookies)
	log.Printf("启动：检测到 cookie 键：%v\n", getMapKeys(baseCookies))

	igneous := fetchIgneous(baseCookies)
	if igneous != "" {
		baseCookies["igneous"] = igneous
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	proxyHandler := NewProxyHandler(baseCookies)
	loggedHandler := loggingMiddleware(proxyHandler)

	log.Printf("启动完成, 代理就绪. 监听端口: %s\n", port)

	if err := http.ListenAndServe(":"+port, loggedHandler); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

func parseCookies(s string) map[string]string {
	cookies := make(map[string]string)
	parts := strings.Split(s, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || !strings.Contains(part, "=") {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		cookies[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	return cookies
}

func getMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
