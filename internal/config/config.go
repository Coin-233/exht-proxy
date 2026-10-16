// Package config loads proxy settings from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
)

const DefaultIPBanMessage = "代理服务器因请求过于频繁被临时封禁, 请在 {countdown} 后重试"

type Config struct {
	WarpEnabled      bool
	WarpProxyURL     string
	WarpRetries      int
	WarpRetrySeconds int
	WarpCLI          string
	IPBanMessage     string
	ExSite           string
	Port             string
	RawCookies       string
	BlockedPaths     []string
	BlockedQueryKeys []string
	BlockedMethods   []string
	ShowLog          bool
	TorrentMode      int
	ArchiverMode     int
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	banMessage := os.Getenv("IP_BAN_MESSAGE")
	if banMessage == "" {
		banMessage = DefaultIPBanMessage
	}
	return Config{
		IPBanMessage:     banMessage,
		WarpEnabled:      os.Getenv("WARP_ENABLED") == "1",
		WarpProxyURL:     envDefault("WARP_PROXY_URL", "socks5://127.0.0.1:40000"),
		WarpCLI:          envDefault("WARP_CLI", "warp-cli"),
		WarpRetries:      positiveInt("WARP_MAX_RETRIES", 3),
		WarpRetrySeconds: positiveInt("WARP_RETRY_SECONDS", 3),
		ExSite:           "https://exhentai.org",
		Port:             port,
		RawCookies:       strings.Trim(os.Getenv("COOKIES"), `"' `),
		ShowLog:          os.Getenv("SHOW_LOG") == "1",
		TorrentMode:      mode("TORRENT_MODE"),
		ArchiverMode:     mode("ARCHIVER_MODE"),
		BlockedPaths: list("BLOCKED_PATHS", []string{
			"archiver.php", "mytags", "repo/torrent_post.php", "gallerytorrents.php", "uconfig.php", "favorites.php",
		}, func(s string) string { return strings.TrimLeft(s, "/") }),
		BlockedQueryKeys: list("BLOCKED_QUERY_KEYS", []string{"report", "act", "inline_set"}, strings.ToLower),
		BlockedMethods:   list("BLOCKED_METHODS", []string{"rategallery", "votecomment", "favorite", "taggallery"}, nil),
	}
}

func mode(key string) int {
	value := os.Getenv(key)
	if value == "" {
		return 1
	}
	n, _ := strconv.Atoi(value)
	return n
}

func list(key string, defaults []string, normalize func(string) string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaults
	}
	var values []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			if normalize != nil {
				item = normalize(item)
			}
			values = append(values, item)
		}
	}
	return values
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func positiveInt(key string, fallback int) int {
	n, err := strconv.Atoi(os.Getenv(key))
	if err != nil || n < 1 {
		return fallback
	}
	return n
}
