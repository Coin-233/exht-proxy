package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	"exht-proxy/internal/config"
)

type warpTransport struct {
	direct, warp http.RoundTripper
	mu           sync.Mutex
	active       bool
	generation   uint64
	retries      int
	delay        time.Duration
	rotate       func(context.Context) error
}

func NewUpstreamClient(cfg config.Config) (*http.Client, error) {
	direct := http.DefaultTransport.(*http.Transport).Clone()
	client := &http.Client{Transport: direct, Timeout: 60 * time.Second}
	if !cfg.WarpEnabled {
		return client, nil
	}
	endpoint, err := url.Parse(cfg.WarpProxyURL)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "socks5" && endpoint.Scheme != "socks5h") {
		return nil, fmt.Errorf("WARP_PROXY_URL 必须为 socks5://host:port")
	}
	cli, err := ensureWarpCLI(context.Background(), cfg.WarpCLI)
	if err != nil {
		return nil, err
	}
	cfg.WarpCLI = cli
	if err := checkWarpProxyMode(context.Background(), cfg.WarpCLI); err != nil {
		return nil, err
	}
	tunneled := direct.Clone()
	tunneled.Proxy = http.ProxyURL(endpoint)
	t := &warpTransport{direct: direct, warp: tunneled, retries: cfg.WarpRetries, delay: time.Duration(cfg.WarpRetrySeconds) * time.Second}
	t.rotate = func(ctx context.Context) error {
		if err := checkWarpProxyMode(ctx, cfg.WarpCLI); err != nil {
			return err
		}
		for _, action := range []string{"disconnect", "connect"} {
			if _, err := warpCommand(ctx, cfg.WarpCLI, action); err != nil {
				return err
			}
		}
		timer := time.NewTimer(t.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
	client.Transport = t
	return client, nil
}

func warpCommand(ctx context.Context, binary string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, append([]string{"--accept-tos"}, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("WARP %v 失败: %w: %s", args, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func checkWarpProxyMode(ctx context.Context, binary string) error {
	output, err := warpCommand(ctx, binary, "settings")
	if err != nil {
		return fmt.Errorf("WARP 服务检查失败; CLI 已就绪，但还需要运行 warp-svc 并配置 proxy 模式: %w", err)
	}
	if !strings.Contains(strings.ToLower(output), "warpproxy") {
		return fmt.Errorf("WARP 必须预先配置为 proxy 模式；为避免影响系统网络，程序不会修改 WARP 模式")
	}
	return nil
}

func (t *warpTransport) route() (http.RoundTripper, uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.active {
		return t.warp, t.generation
	}
	return t.direct, t.generation
}

func (t *warpTransport) switchRoute(ctx context.Context, generation uint64) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.generation != generation {
		return nil
	}
	if err := t.rotate(ctx); err != nil {
		return err
	}
	t.active = true
	t.generation++
	log.Printf("WARP 出口已重连(第 %d 次切换，不保证公网 IP 改变)", t.generation)
	return nil
}

type bufferedBody struct {
	io.Reader
	io.Closer
}

func (t *warpTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		route, generation := t.route()
		resp, err := route.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		contentType := strings.ToLower(resp.Header.Get("Content-Type"))
		if contentType != "" && !strings.HasPrefix(contentType, "text/") && !strings.Contains(contentType, "json") {
			return resp, nil
		}
		reader := bufio.NewReaderSize(resp.Body, 8192)
		prefix, _ := reader.Peek(8192)
		resp.Body = &bufferedBody{Reader: reader, Closer: resp.Body}
		plain := strings.ToLower(strings.Join(strings.Fields(banTextTags.ReplaceAllString(string(prefix), " ")), " "))
		if !strings.Contains(plain, "this ip address has been temporarily banned due to an excessive request rate") || attempt >= t.retries {
			return resp, nil
		}
		if err := t.switchRoute(req.Context(), generation); err != nil {
			log.Printf("WARP 切换失败，返回上游响应: %v", err)
			return resp, nil
		}
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			return resp, nil
		}
		resp.Body.Close()
		if req.Body != nil && req.Body != http.NoBody {
			if req.GetBody == nil {
				return nil, fmt.Errorf("无法重放请求体")
			}
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req = req.Clone(req.Context())
			req.Body = body
		}
	}
}
