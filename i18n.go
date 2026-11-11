package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

var i18nRaw = map[string]map[string]string{}
var i18nMerged = map[string]map[string]string{}
var i18nNested = map[string]map[string]interface{}{}

var i18nOverrides = map[string]map[string]string{
	"zh-cn": {
		"ui.button.readGame":          "从游戏读取",
		"ui.button.fromUrl":           "从URL读取",
		"ui.button.switchAccount":     "切换账号",
		"ui.hint.init":                "点击“从游戏读取”自动获取；或点击“从URL读取”粘贴包含 authkey 的链接",
		"ui.urlDialog.hint":           "粘贴包含 authkey 的调频记录链接",
		"ui.urlDialog.placeholder":    "请粘贴带有 authkey 的调频记录链接",
		"ui.setting.resources":        "资源",
		"ui.setting.fetchFullHistory": "完整读取记录（默认增量读取）",
		"ui.about.version":            "版本",
		"ui.setting.resourceVersion":  "物品数据库版本",
		"ui.setting.resourceUpdated":  "上次检查时间",
		"ui.setting.checkResources":   "检查上游数据更新",
		"ui.setting.quit":             "退出应用",
		"ui.setting.quitHint":         "退出应用并结束本地服务",
		"ui.resource.checking":        "正在检查上游数据资源…",
		"ui.resource.updated":         "数据资源已更新",
		"ui.resource.upToDate":        "数据资源已是最新",
		"ui.resource.failed":          "检查更新失败，继续使用本地缓存",
		"ui.common.close":             "关闭",
		"ui.common.switch":            "切换",
		"ui.data.currentPity":         "已垫抽数",
		"ui.data.noRecords":           "暂无 S 级记录",
	},
	"zh-tw": {
		"ui.button.readGame":          "從遊戲讀取",
		"ui.button.fromUrl":           "從URL讀取",
		"ui.button.switchAccount":     "切換帳號",
		"ui.hint.init":                "點擊「從遊戲讀取」自動獲取；或點擊「從URL讀取」粘貼包含 authkey 的鏈接",
		"ui.urlDialog.hint":           "粘貼包含 authkey 的調頻記錄鏈接",
		"ui.urlDialog.placeholder":    "請粘貼帶有 authkey 的調頻記錄鏈接",
		"ui.setting.resources":        "資源",
		"ui.setting.fetchFullHistory": "完整讀取記錄（預設增量讀取）",
		"ui.about.version":            "版本",
		"ui.setting.resourceVersion":  "物品數據庫版本",
		"ui.setting.resourceUpdated":  "上次檢查時間",
		"ui.setting.checkResources":   "檢查上游數據更新",
		"ui.setting.quit":             "退出應用",
		"ui.setting.quitHint":         "退出應用並結束本地服務",
		"ui.resource.checking":        "正在檢查上游數據資源…",
		"ui.resource.updated":         "數據資源已更新",
		"ui.resource.upToDate":        "數據資源已是最新",
		"ui.resource.failed":          "檢查更新失敗，繼續使用本地緩存",
		"ui.common.close":             "關閉",
		"ui.common.switch":            "切換",
		"ui.data.currentPity":         "已墊抽數",
		"ui.data.noRecords":           "暫無 S 級記錄",
	},
	"en-us": {
		"ui.button.readGame":          "Read from game",
		"ui.button.fromUrl":           "Read from URL",
		"ui.button.switchAccount":     "Switch account",
		"ui.hint.init":                "Click \"Read from game\" to auto-detect, or click \"Read from URL\" to paste a URL containing authkey",
		"ui.urlDialog.hint":           "Paste the signal search history URL containing authkey",
		"ui.urlDialog.placeholder":    "Paste the URL containing authkey",
		"ui.setting.resources":        "Resources",
		"ui.setting.fetchFullHistory": "Read full history (incremental by default)",
		"ui.about.version":            "Version",
		"ui.setting.resourceVersion":  "Item database version",
		"ui.setting.resourceUpdated":  "Last checked",
		"ui.setting.checkResources":   "Check upstream data updates",
		"ui.setting.quit":             "Quit app",
		"ui.setting.quitHint":         "Quit the app and stop the local service",
		"ui.resource.checking":        "Checking upstream data resources…",
		"ui.resource.updated":         "Data resources updated",
		"ui.resource.upToDate":        "Data resources are up to date",
		"ui.resource.failed":          "Update check failed, using local cache",
		"ui.common.close":             "Close",
		"ui.common.switch":            "Switch",
		"ui.data.currentPity":         "Current pity",
		"ui.data.noRecords":           "No S-Rank records",
	},
}

var langMap = [][2]string{{"zh-cn", "简体中文"}, {"zh-tw", "繁體中文"}, {"en-us", "English"}}

func loadI18n() error {
	files := map[string]string{
		"zh-cn": "resources/i18n/zh-cn.json",
		"zh-tw": "resources/i18n/zh-tw.json",
		"en-us": "resources/i18n/en-us.json",
	}
	for lang, path := range files {
		b, err := embedded.ReadFile(path)
		if err != nil {
			return err
		}
		m := map[string]string{}
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		i18nRaw[lang] = m
	}
	for lang := range i18nRaw {
		merged := map[string]string{}
		for k, v := range i18nRaw["zh-cn"] {
			merged[k] = v
		}
		if lang != "zh-tw" {
			for k, v := range i18nRaw["en-us"] {
				if v != "" {
					merged[k] = v
				}
			}
		}
		for k, v := range i18nRaw[lang] {
			if v != "" {
				merged[k] = v
			}
		}
		for k, v := range i18nOverrides[lang] {
			merged[k] = v
		}
		i18nMerged[lang] = merged
		i18nNested[lang] = parseDotted(merged)
	}
	return nil
}

func addProp(obj map[string]interface{}, key string) map[string]interface{} {
	if v, ok := obj[key]; ok {
		if m, ok := v.(map[string]interface{}); ok {
			return m
		}
	}
	m := map[string]interface{}{}
	obj[key] = m
	return m
}

func parseDotted(flat map[string]string) map[string]interface{} {
	root := map[string]interface{}{}
	for key, val := range flat {
		parts := strings.Split(key, ".")
		cur := root
		for i, p := range parts {
			if i == len(parts)-1 {
				cur[p] = val
				break
			}
			cur = addProp(cur, p)
		}
	}
	return root
}

func getI18nNested(lang string) map[string]interface{} {
	if v, ok := i18nNested[lang]; ok {
		return v
	}
	return i18nNested["zh-cn"]
}

func i18nText(lang, key string) string {
	if m, ok := i18nMerged[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if v, ok := i18nMerged["zh-cn"][key]; ok {
		return v
	}
	return key
}

func detectLocale(lang string) string {
	l := strings.ToLower(lang)
	switch {
	case strings.HasPrefix(l, "zh-tw"), strings.HasPrefix(l, "zh-hk"), strings.HasPrefix(l, "zh-hant"):
		return "zh-tw"
	case strings.HasPrefix(l, "zh"):
		return "zh-cn"
	case strings.HasPrefix(l, "en"):
		return "en-us"
	default:
		return "zh-cn"
	}
}

func langMapJSON() [][]string {
	out := [][]string{}
	for _, p := range langMap {
		out = append(out, []string{p[0], p[1]})
	}
	return out
}
