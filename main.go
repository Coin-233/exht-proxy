package main

import (
	"bufio"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
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
	TorrentMode      int
	ArchiverMode     int
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
	InitDB()
	go InitTagDB()

	ShowLog = os.Getenv("SHOW_LOG") == "1"
	if ShowLog {
		log.Println("已开启详细访问日志输出")
	}

	if modeStr := os.Getenv("TORRENT_MODE"); modeStr != "" {
		if mode, err := strconv.Atoi(modeStr); err == nil {
			TorrentMode = mode
		}
	} else {
		TorrentMode = 1
	}

	if modeStr := os.Getenv("ARCHIVER_MODE"); modeStr != "" {
		if mode, err := strconv.Atoi(modeStr); err == nil {
			ArchiverMode = mode
		}
	} else {
		ArchiverMode = 1
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

	// 定期刷新 igneous cookie（每 6 小时），防止长期运行后鉴权失效
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			newIgneous := fetchIgneous(baseCookies)
			if newIgneous != "" {
				proxyHandler.UpdateCookie("igneous", newIgneous)
				log.Printf("igneous cookie 已自动刷新: %s\n", newIgneous)
			} else {
				log.Println("igneous 刷新失败，继续使用原值")
			}
		}
	}()

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      loggedHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("启动完成, 代理就绪. 监听端口: %s\n", port)

	// 优雅关闭：等待 SIGINT / SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("收到退出信号，正在优雅关闭...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("服务器关闭时出错: %v\n", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("服务器启动失败: %v", err)
	}
	log.Println("服务器已关闭")
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
