package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type IconItem struct {
	Kind  string `json:"kind"`
	Icon  string `json:"icon"`
	Image string `json:"image,omitempty"`
}

type iconDoc struct {
	Source string              `json:"source"`
	Base   string              `json:"base"`
	Items  map[string]IconItem `json:"items"`
}

type iconState struct {
	sync.RWMutex
	Items      map[string]IconItem
	CheckedAt  int64
	Source     string
	refreshing bool
}

var iconsState = iconState{Items: map[string]IconItem{}, Source: "bundled"}

const iconCDN = "https://enka.network"
const iconCacheDirName = "icons"

func iconCacheDir() string  { return filepath.Join(dataDir, iconCacheDirName) }
func iconCacheFile() string { return filepath.Join(dataDir, "resources", "enka-icons.json") }
func iconCacheMeta() string { return filepath.Join(dataDir, "resources", "enka-meta.json") }

func loadIcons() error {
	if b, err := os.ReadFile(iconCacheFile()); err == nil {
		var doc iconDoc
		if json.Unmarshal(b, &doc) == nil && len(doc.Items) > 0 {
			iconsState.Lock()
			iconsState.Items = doc.Items
			iconsState.Source = "cache"
			iconsState.Unlock()
			var meta struct{ CheckedAt int64 }
			if mb, err := os.ReadFile(iconCacheMeta()); err == nil {
				_ = json.Unmarshal(mb, &meta)
			}
			iconsState.Lock()
			iconsState.CheckedAt = meta.CheckedAt
			iconsState.Unlock()
			return nil
		}
	}
	b, err := embedded.ReadFile("resources/icons.json")
	if err != nil {
		return err
	}
	var doc iconDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return err
	}
	iconsState.Lock()
	iconsState.Items = doc.Items
	iconsState.Source = "bundled"
	iconsState.Unlock()
	return nil
}

func iconCacheTagPath() string { return filepath.Join(dataDir, "resources", "icon-cache-tag") }

func iconCacheTag() string {
	b, err := os.ReadFile(iconCacheTagPath())
	if err != nil {
		return "0"
	}
	tag := strings.TrimSpace(string(b))
	if tag == "" {
		return "0"
	}
	return tag
}

func bumpIconCacheTag() string {
	tag := fmt.Sprintf("%d", time.Now().UnixNano())
	_ = os.MkdirAll(filepath.Join(dataDir, "resources"), 0755)
	_ = os.WriteFile(iconCacheTagPath(), []byte(tag), 0644)
	return tag
}

func iconInfo() map[string]interface{} {
	iconsState.RLock()
	defer iconsState.RUnlock()
	return map[string]interface{}{"count": len(iconsState.Items), "checkedAt": iconsState.CheckedAt, "source": iconsState.Source, "refreshing": iconsState.refreshing, "cacheTag": iconCacheTag()}
}

func getIconMeta(id string) (IconItem, bool) {
	iconsState.RLock()
	defer iconsState.RUnlock()
	it, ok := iconsState.Items[id]
	return it, ok
}

func buildIconDoc(avatars, weapons map[string]json.RawMessage) iconDoc {
	doc := iconDoc{Source: "https://github.com/EnkaNetwork/API-docs", Base: iconCDN, Items: map[string]IconItem{}}
	for id, raw := range avatars {
		var a struct {
			CircleIcon string `json:"CircleIcon"`
			Image      string `json:"Image"`
		}
		if json.Unmarshal(raw, &a) == nil && a.CircleIcon != "" {
			doc.Items[id] = IconItem{Kind: "character", Icon: a.CircleIcon, Image: a.Image}
		}
	}
	for id, raw := range weapons {
		var w struct {
			ImagePath string `json:"ImagePath"`
		}
		if json.Unmarshal(raw, &w) == nil && w.ImagePath != "" {
			doc.Items[id] = IconItem{Kind: "weapon", Icon: w.ImagePath}
		}
	}
	return doc
}

func refreshIcons() map[string]interface{} {
	iconsState.Lock()
	if iconsState.refreshing {
		iconsState.Unlock()
		info := iconInfo()
		return map[string]interface{}{"ok": false, "busy": true, "icons": info}
	}
	iconsState.refreshing = true
	iconsState.Unlock()

	checkedAt := time.Now().UnixMilli()
	client := &http.Client{Timeout: 20 * time.Second}
	get := func(u string) (map[string]json.RawMessage, error) {
		resp, err := client.Get(u)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		var m map[string]json.RawMessage
		return m, json.NewDecoder(resp.Body).Decode(&m)
	}
	base := "https://raw.githubusercontent.com/EnkaNetwork/API-docs/master/store/zzz/"
	avatars, err1 := get(base + "avatars.json")
	weapons, err2 := get(base + "weapons.json")
	if err1 != nil || err2 != nil {
		iconsState.Lock()
		iconsState.refreshing = false
		iconsState.Unlock()
		e := err1
		if e == nil {
			e = err2
		}
		return map[string]interface{}{"ok": false, "error": e.Error(), "icons": iconInfo()}
	}
	doc := buildIconDoc(avatars, weapons)
	if len(doc.Items) == 0 {
		iconsState.Lock()
		iconsState.refreshing = false
		iconsState.Unlock()
		return map[string]interface{}{"ok": false, "error": "empty icon index", "icons": iconInfo()}
	}
	_ = os.MkdirAll(filepath.Join(dataDir, "resources"), 0755)
	b, _ := json.Marshal(doc)
	_ = os.WriteFile(iconCacheFile(), b, 0644)
	iconsState.Lock()
	changed := len(iconsState.Items) != len(doc.Items)
	iconsState.Items = doc.Items
	iconsState.CheckedAt = checkedAt
	iconsState.Source = "upstream"
	iconsState.refreshing = false
	iconsState.Unlock()
	meta, _ := json.Marshal(map[string]interface{}{"checkedAt": checkedAt, "count": len(doc.Items)})
	_ = os.WriteFile(iconCacheMeta(), meta, 0644)
	return map[string]interface{}{"ok": true, "updated": changed, "icons": iconInfo()}
}

func contentTypeOfExt(ext string) string {
	if ext == ".webp" {
		return "image/webp"
	}
	return "image/png"
}

func iconRequestPath(icon string) string {
	if strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://") {
		return icon
	}
	return iconCDN + icon
}

var iconSem = make(chan struct{}, 3)
var iconPending sync.Map

type iconSource struct {
	url string
	ext string
}

func ensureIcon(id string) ([]byte, string, error) {
	// Nanoka's character art is a full-body illustration, not a headshot.
	// Prefer Enka's CircleIcon for character/weapon avatars; keep Nanoka as
	// the fallback, especially for Bangboo, which Enka does not cover.
	candidates := []iconSource{}
	if item, ok := getIconMeta(id); ok {
		ext := filepath.Ext(item.Icon)
		if ext != ".webp" {
			ext = ".png"
		}
		candidates = append(candidates, iconSource{url: iconRequestPath(item.Icon), ext: ext})
	}
	if icon := getItemIcon(id); icon != "" {
		candidates = append(candidates, iconSource{url: "https://static.nanoka.cc/assets/zzz/" + icon + ".webp", ext: ".webp"})
	}
	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("icon not found")
	}

	if v, ok := iconPending.Load(id); ok {
		<-v.(chan struct{})
		if ev, ok := iconErr.Load(id); ok && ev != nil {
			return nil, "", ev.(error)
		}
		for _, src := range candidates {
			if b, err := os.ReadFile(filepath.Join(iconCacheDir(), id+src.ext)); err == nil {
				return b, contentTypeOfExt(src.ext), nil
			}
		}
		return nil, "", fmt.Errorf("icon unavailable")
	}

	done := make(chan struct{})
	iconPending.Store(id, done)
	var resultErr error
	var resultBytes []byte
	var resultExt string
	func() {
		iconSem <- struct{}{}
		defer func() { <-iconSem }()
		client := &http.Client{Timeout: 20 * time.Second}
		for _, src := range candidates {
			// Re-check cache for this source (another process may have filled it).
			if b, err := os.ReadFile(filepath.Join(iconCacheDir(), id+src.ext)); err == nil {
				resultBytes = b
				resultExt = src.ext
				resultErr = nil
				return
			}
			resp, err := client.Get(src.url)
			if err != nil {
				resultErr = err
				continue
			}
			b, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				resultErr = err
				continue
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				resultErr = fmt.Errorf("HTTP %d", resp.StatusCode)
				continue
			}
			if len(b) == 0 {
				resultErr = fmt.Errorf("empty image")
				continue
			}
			_ = os.MkdirAll(iconCacheDir(), 0755)
			if err := os.WriteFile(filepath.Join(iconCacheDir(), id+src.ext), b, 0644); err != nil {
				resultErr = err
				continue
			}
			resultBytes = b
			resultExt = src.ext
			resultErr = nil
			return
		}
	}()
	iconPending.Delete(id)
	if resultErr != nil {
		iconErr.Store(id, resultErr)
	} else {
		iconErr.Delete(id)
	}
	close(done)
	if resultErr != nil {
		return nil, "", resultErr
	}
	return resultBytes, contentTypeOfExt(resultExt), nil
}

var iconErr sync.Map

func clearIconCache() bool {
	entries, err := os.ReadDir(iconCacheDir())
	if err != nil {
		return false
	}
	for _, e := range entries {
		_ = os.Remove(filepath.Join(iconCacheDir(), e.Name()))
	}
	bumpIconCacheTag()
	return true
}
