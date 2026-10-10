// Package config loads proxy settings from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
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
	return Config{
		ExSite:       "https://exhentai.org",
		Port:         port,
		RawCookies:   strings.Trim(os.Getenv("COOKIES"), `"' `),
		ShowLog:      os.Getenv("SHOW_LOG") == "1",
		TorrentMode:  mode("TORRENT_MODE"),
		ArchiverMode: mode("ARCHIVER_MODE"),
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
