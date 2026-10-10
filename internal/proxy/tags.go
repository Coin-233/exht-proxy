package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// 翻译标签用
type TagItem struct {
	Name  string `json:"n"`
	Intro string `json:"i"`
}

func (h *ProxyHandler) InitTagDB() {
	fmt.Println("正在拉取 EhTagTranslation 标签数据库...")
	resp, err := http.Get("https://cdn.jsdelivr.net/gh/EhTagTranslation/DatabaseReleases/db.text.json")
	if err != nil {
		fmt.Println("获取标签数据库失败:", err)
		return
	}
	defer resp.Body.Close()

	var db struct {
		Data []struct {
			Namespace string `json:"namespace"`
			Data      map[string]struct {
				Name  string `json:"name"`
				Intro string `json:"intro"`
			} `json:"data"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&db); err != nil {
		fmt.Println("解析标签数据库失败:", err)
		return
	}

	tagMap := make(map[string]TagItem)
	for _, ns := range db.Data {
		for eng, tagData := range ns.Data {
			item := TagItem{Name: tagData.Name, Intro: tagData.Intro}
			// 保存 命名空间:标签
			tagMap[ns.Namespace+":"+eng] = item
			// 保存裸标签做后备
			if _, exists := tagMap[eng]; !exists {
				tagMap[eng] = item
			}
		}
	}

	// 补充左侧命名空间分类的汉化
	nsMap := map[string]string{
		"artist": "画师", "character": "角色", "female": "女性", "male": "男性",
		"parody": "原作", "group": "团队", "mixed": "混合", "language": "语言",
		"reclass": "重新分类", "other": "其他", "cosplayer": "Coser",
	}
	for k, v := range nsMap {
		tagMap["namespace:"+k] = TagItem{Name: v, Intro: ""}
	}

	data, _ := json.Marshal(tagMap)
	h.tagMu.Lock()
	h.tagDBJSON = data
	h.tagMu.Unlock()
	fmt.Printf("标签数据库加载完成，共汉化 %d 条标签\n", len(tagMap))
}
