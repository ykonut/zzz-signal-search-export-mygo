package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type GachaType struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type ItemInfo struct {
	Name     string `json:"name"`
	ItemType string `json:"item_type"`
	RankType int    `json:"rank_type"`
	Icon     string `json:"icon,omitempty"`
}

type resourceState struct {
	sync.RWMutex
	GachaTypes map[string][]GachaType
	IdJson     map[string]map[string]ItemInfo
	IdVersion  string
	UpdatedAt  int64
	CheckedAt  int64
	Source     string
	LastError  string
	refreshing bool
}

var resState = resourceState{GachaTypes: map[string][]GachaType{}, IdJson: map[string]map[string]ItemInfo{}, Source: "bundled"}

const repoName = "earthjasonlin/zzz-signal-search-export"

var resourceSources = map[string][]string{
	"idJson": {
		"https://raw.githubusercontent.com/" + repoName + "/main/src/idJson.json",
		"https://cdn.jsdelivr.net/gh/" + repoName + "@main/src/idJson.json",
	},
	"gachaType": {
		"https://raw.githubusercontent.com/" + repoName + "/main/src/gachaType.json",
		"https://cdn.jsdelivr.net/gh/" + repoName + "@main/src/gachaType.json",
	},
}

type nanokaManifest struct {
	Zzz struct {
		Live   string `json:"live"`
		Latest string `json:"latest"`
	} `json:"zzz"`
}

type nanokaItem struct {
	Rank     int    `json:"rank"`
	Type     int    `json:"type"`
	Icon     string `json:"icon"`
	Code     string `json:"code"`
	Codename string `json:"codename"`
	Zh       string `json:"zh"`
	En       string `json:"en"`
	Ja       string `json:"ja"`
	Ko       string `json:"ko"`
}

var nanokaTypeNames = map[string]map[string]string{
	"zh-cn": {"character": "代理人", "weapon": "音擎", "bangboo": "邦布"},
	"zh-tw": {"character": "代理人", "weapon": "音擎", "bangboo": "邦布"},
	"en-us": {"character": "Agents", "weapon": "W-Engines", "bangboo": "Bangboo"},
}

func nanokaUserName(item nanokaItem, lang string) string {
	if lang == "en-us" {
		if item.En != "" {
			return item.En
		}
		return item.Code
	}
	if item.Zh != "" {
		return item.Zh
	}
	return item.En
}

func nanokaIconBase(p string) string {
	s := strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(s, ".png")
	s = strings.TrimSuffix(s, ".webp")
	s = strings.TrimSuffix(s, ".jpg")
	return s
}

func getItemIcon(id string) string {
	resState.RLock()
	defer resState.RUnlock()
	for _, lang := range []string{"zh-cn", "en-us", "zh-tw"} {
		if m, ok := resState.IdJson[lang]; ok {
			if item, ok := m[id]; ok && item.Icon != "" {
				return item.Icon
			}
		}
	}
	return ""
}

// fetchNanokaIDJson uses Nanoka's public static data as a direct item-data source.
// It converts Nanoka's raw character/weapon/bangboo JSON into the normalized
// idJson shape used by the rest of the app.
func fetchNanokaIDJson() (map[string]map[string]ItemInfo, string, []byte, error) {
	manifestBytes, err := fetchJSONURL([]string{"https://static.nanoka.cc/manifest.json"})
	if err != nil {
		return nil, "", nil, err
	}
	var manifest nanokaManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, "", nil, err
	}
	version := manifest.Zzz.Live
	if version == "" {
		version = manifest.Zzz.Latest
	}
	if version == "" {
		return nil, "", nil, fmt.Errorf("nanoka manifest has no zzz version")
	}
	base := "https://static.nanoka.cc/zzz/" + url.PathEscape(version)
	fetch := func(name string) (map[string]nanokaItem, error) {
		b, err := fetchJSONURL([]string{base + "/" + name})
		if err != nil {
			return nil, err
		}
		out := map[string]nanokaItem{}
		if err := json.Unmarshal(b, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	characters, err := fetch("character.json")
	if err != nil {
		return nil, "", nil, err
	}
	weapons, err := fetch("weapon.json")
	if err != nil {
		return nil, "", nil, err
	}
	bangboos, err := fetch("bangboo.json")
	if err != nil {
		return nil, "", nil, err
	}

	out := map[string]map[string]ItemInfo{}
	for _, lang := range []string{"zh-cn", "zh-tw", "en-us"} {
		out[lang] = map[string]ItemInfo{}
		// Preserve existing traditional-Chinese names when Nanoka only provides zh/en.
		resState.RLock()
		existing := resState.IdJson[lang]
		resState.RUnlock()
		add := func(items map[string]nanokaItem, kind string) {
			for id, item := range items {
				name := nanokaUserName(item, lang)
				if lang == "zh-tw" && existing != nil {
					if old, ok := existing[id]; ok && old.Name != "" {
						name = old.Name
					}
				}
				if name == "" {
					name = id
				}
				rank := item.Rank
				if rank == 0 && existing != nil {
					if old, ok := existing[id]; ok {
						rank = old.RankType
					}
				}
				out[lang][id] = ItemInfo{Name: name, ItemType: nanokaTypeNames[lang][kind], RankType: rank, Icon: nanokaIconBase(item.Icon)}
			}
		}
		add(characters, "character")
		add(weapons, "weapon")
		add(bangboos, "bangboo")
	}
	doc := map[string]interface{}{"version": version}
	for lang, items := range out {
		doc[lang] = items
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, "", nil, err
	}
	return out, version, b, nil
}

func resourceCache(name string) string { return filepath.Join(dataDir, "resources", name) }

func parseGachaTypeBytes(b []byte) (map[string][]GachaType, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := map[string][]GachaType{}
	for _, item := range raw {
		var pair []json.RawMessage
		if err := json.Unmarshal(item, &pair); err != nil || len(pair) < 2 {
			continue
		}
		var lang string
		var types []GachaType
		_ = json.Unmarshal(pair[0], &lang)
		_ = json.Unmarshal(pair[1], &types)
		if lang != "" {
			out[lang] = types
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("invalid gachaType")
	}
	return out, nil
}

func parseIdJsonBytes(b []byte) (map[string]map[string]ItemInfo, string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, "", err
	}
	idv := ""
	if v, ok := raw["version"]; ok {
		_ = json.Unmarshal(v, &idv)
	}
	out := map[string]map[string]ItemInfo{}
	for lang, data := range raw {
		if lang == "version" {
			continue
		}
		m := map[string]ItemInfo{}
		if err := json.Unmarshal(data, &m); err == nil {
			out[lang] = m
		}
	}
	if len(out) == 0 {
		return nil, "", fmt.Errorf("invalid idJson")
	}
	return out, idv, nil
}

func applyResourceData(idJson map[string]map[string]ItemInfo, idVersion string, gacha map[string][]GachaType, source string, updatedAt int64) {
	resState.Lock()
	defer resState.Unlock()
	if idJson != nil {
		resState.IdJson = idJson
	}
	if idVersion != "" {
		resState.IdVersion = idVersion
	}
	if gacha != nil {
		resState.GachaTypes = gacha
	}
	if source != "" {
		resState.Source = source
	}
	if updatedAt > 0 {
		resState.UpdatedAt = updatedAt
	}
}

func loadBundledResources() error {
	idBytes, err := embedded.ReadFile("resources/idJson.json")
	if err != nil {
		return err
	}
	gtBytes, err := embedded.ReadFile("resources/gachaType.json")
	if err != nil {
		return err
	}
	id, ver, err := parseIdJsonBytes(idBytes)
	if err != nil {
		return err
	}
	gacha, err := parseGachaTypeBytes(gtBytes)
	if err != nil {
		return err
	}
	applyResourceData(id, ver, gacha, "bundled", 0)
	return nil
}

func loadCachedResources() bool {
	idBytes, err1 := os.ReadFile(resourceCache("idJson.json"))
	gtBytes, err2 := os.ReadFile(resourceCache("gachaType.json"))
	if err1 != nil || err2 != nil {
		return false
	}
	id, ver, err := parseIdJsonBytes(idBytes)
	if err != nil {
		return false
	}
	gacha, err := parseGachaTypeBytes(gtBytes)
	if err != nil {
		return false
	}
	var meta struct{ UpdatedAt, CheckedAt int64 }
	if b, err := os.ReadFile(filepath.Join(dataDir, "resources", "meta.json")); err == nil {
		_ = json.Unmarshal(b, &meta)
	}
	// Older caches have no icon field; force one refresh so Nanoka icon names are available.
	hasIcon := false
	for _, it := range id["zh-cn"] {
		if it.Icon != "" {
			hasIcon = true
			break
		}
	}
	if !hasIcon {
		meta.CheckedAt = 0
	}
	applyResourceData(id, ver, gacha, "cache", meta.UpdatedAt)
	resState.Lock()
	resState.CheckedAt = meta.CheckedAt
	resState.Unlock()
	return true
}

func initResources() error {
	_ = os.MkdirAll(filepath.Join(dataDir, "resources"), 0755)
	if !loadCachedResources() {
		if err := loadBundledResources(); err != nil {
			return err
		}
	}
	return nil
}

func fetchJSONURL(urls []string) ([]byte, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	var last error
	for _, u := range urls {
		req, _ := http.NewRequest("GET", u, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 ZZZ-Signal-Search-Export")
		resp, err := client.Do(req)
		if err != nil {
			last = err
			continue
		}
		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			last = err
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		return b, nil
	}
	if last == nil {
		last = fmt.Errorf("fetch failed")
	}
	return nil, last
}

func refreshResources() map[string]interface{} {
	resState.Lock()
	if resState.refreshing {
		info := resourceInfoLocked()
		resState.Unlock()
		info["ok"] = false
		info["busy"] = true
		return info
	}
	resState.refreshing = true
	resState.Unlock()

	checkedAt := time.Now().UnixMilli()
	info, changed, err := doRefreshResources(checkedAt)
	resState.Lock()
	resState.refreshing = false
	if err != nil {
		resState.LastError = err.Error()
		resState.CheckedAt = checkedAt
	}
	resState.Unlock()
	if err != nil {
		out := resourceInfo()
		out["ok"] = false
		out["error"] = err.Error()
		return out
	}
	out := info
	out["ok"] = true
	out["updated"] = changed
	return out
}

func doRefreshResources(checkedAt int64) (map[string]interface{}, bool, error) {
	// Nanoka is the direct item-data source. Fall back to the upstream
	// repository when Nanoka is unreachable or returns invalid data.
	id, ver, idBytes, err := fetchNanokaIDJson()
	source := "nanoka"
	if err != nil {
		idBytes, err = fetchJSONURL(resourceSources["idJson"])
		if err != nil {
			return nil, false, err
		}
		id, ver, err = parseIdJsonBytes(idBytes)
		if err != nil {
			return nil, false, err
		}
		source = "upstream"
	}

	// Banner names are not part of Nanoka's item files; use the upstream
	// gachaType file, with the bundled copy as a final fallback.
	gtBytes, gtErr := fetchJSONURL(resourceSources["gachaType"])
	var gacha map[string][]GachaType
	if gtErr == nil {
		gacha, gtErr = parseGachaTypeBytes(gtBytes)
	}
	if gtErr != nil {
		gtBytes, _ = embedded.ReadFile("resources/gachaType.json")
		gacha, gtErr = parseGachaTypeBytes(gtBytes)
	}
	if gtErr != nil {
		return nil, false, gtErr
	}

	resState.RLock()
	changed := ver != resState.IdVersion
	resState.RUnlock()

	_ = os.MkdirAll(filepath.Join(dataDir, "resources"), 0755)
	_ = os.WriteFile(resourceCache("idJson.json"), idBytes, 0644)
	_ = os.WriteFile(resourceCache("gachaType.json"), gtBytes, 0644)
	applyResourceData(id, ver, gacha, source, checkedAt)
	resState.Lock()
	resState.CheckedAt = checkedAt
	resState.LastError = ""
	resState.Unlock()
	meta := map[string]interface{}{"updatedAt": checkedAt, "checkedAt": checkedAt, "idVersion": ver, "source": source}
	b, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(filepath.Join(dataDir, "resources", "meta.json"), b, 0644)
	return resourceInfo(), changed, nil
}

func resourceInfoLocked() map[string]interface{} {
	return map[string]interface{}{
		"idVersion": resState.IdVersion, "updatedAt": resState.UpdatedAt, "checkedAt": resState.CheckedAt,
		"source": resState.Source, "refreshing": resState.refreshing, "lastError": resState.LastError,
	}
}

func resourceInfo() map[string]interface{} {
	resState.RLock()
	defer resState.RUnlock()
	return resourceInfoLocked()
}

func getGachaTypes(lang string) []GachaType {
	resState.RLock()
	defer resState.RUnlock()
	if v, ok := resState.GachaTypes[lang]; ok {
		return v
	}
	return resState.GachaTypes["zh-cn"]
}

func getItemMap(lang string) map[string]ItemInfo {
	resState.RLock()
	defer resState.RUnlock()
	if m, ok := resState.IdJson[lang]; ok {
		return m
	}
	return resState.IdJson["zh-cn"]
}
