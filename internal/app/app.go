package app

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"exht-proxy/internal/config"
	"exht-proxy/internal/proxy"
	"exht-proxy/internal/storage"
	"github.com/joho/godotenv"
)

func loggingMiddleware(next http.Handler, showLog bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if showLog {
			log.Printf("[访问日志] %s - %s %s - 耗时: %v", r.RemoteAddr, r.Method, r.URL.Path, time.Since(start))
		}
	})
}

func Run() error {
	_ = godotenv.Load()
	cfg := config.Load()
	store, err := storage.Open("./proxy_stats.db")
	if err != nil {
		return err
	}
	defer store.Close()
	if cfg.ShowLog {
		log.Println("已开启详细访问日志输出")
	}

	// 获取 Igneous Cookie
	baseCookies := parseCookies(cfg.RawCookies)
	log.Printf("启动：检测到 cookie 键：%v\n", getMapKeys(baseCookies))

	igneous := proxy.FetchIgneous(cfg.ExSite, baseCookies)
	if igneous != "" {
		baseCookies["igneous"] = igneous
	}

	port := cfg.Port
	proxyHandler := proxy.NewProxyHandler(cfg, baseCookies, store)
	go proxyHandler.InitTagDB()
	loggedHandler := loggingMiddleware(proxyHandler, cfg.ShowLog)

	refreshCookies := make(map[string]string, len(baseCookies))
	for key, value := range baseCookies {
		refreshCookies[key] = value
	}
	done := make(chan struct{})
	defer close(done)
	// 定期刷新 igneous cookie(每 6 小时), 防止长期运行后鉴权失效
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
			}
			newIgneous := proxy.FetchIgneous(cfg.ExSite, refreshCookies)
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
	defer signal.Stop(quit)
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-done:
			return
		case <-quit:
		}
		log.Println("收到退出信号，正在优雅关闭...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("服务器关闭时出错: %v\n", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	<-shutdownDone
	log.Println("服务器已关闭")
	return nil
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
