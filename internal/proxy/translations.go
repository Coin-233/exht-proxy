package proxy

import (
	"bufio"
	"io"
	"log"
	"os"
	"strings"
)

func (h *ProxyHandler) loadTranslations() {
	h.translations = make(map[string]string)
	h.jsTranslations = make(map[string]string)

	var source io.Reader = strings.NewReader(defaultTranslations)
	file, err := os.Open("translations.txt")
	if err == nil {
		defer file.Close()
		source = file
	} else if !os.IsNotExist(err) {
		log.Printf("警告: 无法读取 translations.txt, 使用内置汉化字典: %v", err)
	}
	scanner := bufio.NewScanner(source)
	var currentTarget map[string]string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 忽略空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if line == "[HTML]" {
			currentTarget = h.translations
			continue
		} else if line == "[JS]" {
			currentTarget = h.jsTranslations
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
		log.Printf("成功加载汉化字典: HTML(%d条) JS(%d条)\n", len(h.translations), len(h.jsTranslations))
	}
}
