package proxy

import (
	"html"
	"regexp"
	"strings"

	"exht-proxy/internal/config"
)

var (
	banTextTags = regexp.MustCompile(`<[^>]*>`)
	banExpiry   = regexp.MustCompile(`(?i)the ban expires in\s*(?:(\d+)\s*hours?\s*(?:,|and)?\s*)?(?:(\d+)\s*minutes?\s*(?:,|and)?\s*)?(?:(\d+)\s*seconds?)?`)
)

func (h *ProxyHandler) ipBanMessage(content string) (string, bool) {
	plain := strings.Join(strings.Fields(html.UnescapeString(banTextTags.ReplaceAllString(content, " "))), " ")
	if !strings.Contains(strings.ToLower(plain), "this ip address has been temporarily banned due to an excessive request rate") {
		return "", false
	}
	match := banExpiry.FindStringSubmatch(plain)
	if len(match) == 0 || (match[1] == "" && match[2] == "" && match[3] == "") {
		return plain, true
	}
	units := []string{"小时", "分钟", "秒"}
	var parts []string
	for i := 1; i <= 3; i++ {
		if match[i] != "" {
			parts = append(parts, match[i]+units[i-1])
		} else {
			match[i] = "0"
		}
	}
	message := h.config.IPBanMessage
	if message == "" {
		message = config.DefaultIPBanMessage
	}
	return strings.NewReplacer(
		"{countdown}", strings.Join(parts, " "),
		"{hours}", match[1], "{minutes}", match[2], "{seconds}", match[3],
	).Replace(message), true
}
