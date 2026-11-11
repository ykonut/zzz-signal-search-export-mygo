package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func readBody(r *http.Request, dst interface{}) error {
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, dst)
}

func bootstrapData() map[string]interface{} {
	cfg := getConfig()
	out := map[string]interface{}{
		"ok": true, "version": appVersion,
		"config": configForJSON(), "i18n": getI18nNested(cfg.Lang),
		"langMap": langMapJSON(), "resources": resourceInfo(), "icons": iconInfo(),
	}
	for k, v := range serializeData() {
		out[k] = v
	}
	return out
}

func newMux() http.Handler {
	webFS, _ := fs.Sub(embedded, "web")
	fileServer := http.FileServer(http.FS(webFS))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			apiHandler(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		fileServer.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/icon/") {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		mux.ServeHTTP(w, r)
	})
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	pathname := r.URL.Path
	method := r.Method
	switch {
	case pathname == "/api/bootstrap":
		readData()
		writeJSON(w, 200, bootstrapData())
	case pathname == "/api/events":
		hub.sseHandler(w, r)
	case pathname == "/api/fetch" && method == "POST":
		var body struct {
			URL string `json:"url"`
		}
		if err := readBody(r, &body); err != nil {
			writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		res, err := fetchData(body.URL)
		if err != nil {
			writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		res["ok"] = true
		writeJSON(w, 200, res)
	case pathname == "/api/fetch-game" && method == "POST":
		hit := findGachaURL()
		if hit == nil {
			writeJSON(w, 200, map[string]interface{}{"ok": false, "error": "未在游戏缓存中找到抽卡链接。请先在游戏内打开一次「调频记录」页面，再点击「从游戏读取」。"})
			return
		}
		hub.sendMsg("已从"+hit.Source+"读取到抽卡链接，开始获取数据…", "STATUS")
		res, err := fetchData(hit.URL)
		if err != nil {
			writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		res["ok"] = true
		res["source"] = hit.Source
		writeJSON(w, 200, res)
	case pathname == "/api/change-uid" && method == "POST":
		var body struct {
			UID interface{} `json:"uid"`
		}
		if err := readBody(r, &body); err != nil {
			writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		readData()
		_ = changeCurrent(anyToString(body.UID))
		writeJSON(w, 200, map[string]interface{}{"ok": true, "current": currentUID()})
	case pathname == "/api/delete" && method == "POST":
		var body struct {
			UID     interface{} `json:"uid"`
			Deleted bool        `json:"deleted"`
		}
		if err := readBody(r, &body); err != nil {
			writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		readData()
		_ = deleteData(anyToString(body.UID), body.Deleted)
		out := serializeData()
		out["ok"] = true
		writeJSON(w, 200, out)
	case pathname == "/api/config" && method == "POST":
		var body struct {
			Key   string      `json:"key"`
			Value interface{} `json:"value"`
		}
		if err := readBody(r, &body); err != nil {
			writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		if !setConfigValue(body.Key, body.Value) {
			writeJSON(w, 400, map[string]interface{}{"ok": false, "error": "invalid key"})
			return
		}
		_ = saveConfig()
		out := map[string]interface{}{"ok": true, "config": configForJSON()}
		if body.Key == "lang" {
			out["i18n"] = getI18nNested(getConfig().Lang)
		}
		writeJSON(w, 200, out)
	case pathname == "/api/import/uigf" && method == "POST":
		var body struct {
			Content string `json:"content"`
		}
		if err := readBody(r, &body); err != nil {
			writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		readData()
		if err := importUIGF([]byte(body.Content)); err != nil {
			writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		out := serializeData()
		out["ok"] = true
		writeJSON(w, 200, out)
	case pathname == "/api/export/excel" && method == "GET":
		readData()
		uid := r.URL.Query().Get("uid")
		ds := getCurrentData()
		if uid != "" {
			dataMu.RLock()
			ds = dataMap[uid]
			dataMu.RUnlock()
		}
		if ds == nil {
			writeJSON(w, 400, map[string]interface{}{"ok": false, "error": "no data"})
			return
		}
		buf, filename, err := buildExcel(ds, getConfig().Lang)
		if err != nil {
			writeJSON(w, 500, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", contentDisposition(filename))
		w.Header().Set("Content-Length", strconv.Itoa(len(buf)))
		_, _ = w.Write(buf)
	case pathname == "/api/export/uigf" && method == "GET":
		readData()
		uids := []string{}
		for _, u := range strings.Split(r.URL.Query().Get("uids"), ",") {
			if strings.TrimSpace(u) != "" {
				uids = append(uids, strings.TrimSpace(u))
			}
		}
		b, filename, err := exportUIGF(uids)
		if err != nil {
			writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", contentDisposition(filename))
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		_, _ = w.Write(b)
	case pathname == "/api/resources/refresh" && method == "POST":
		resInfo := refreshResources()
		iconInfo := refreshIcons()
		writeJSON(w, 200, map[string]interface{}{"ok": true, "resources": resInfo, "icons": iconInfo})
	case pathname == "/api/icons/clear" && method == "POST":
		ok := clearIconCache()
		writeJSON(w, 200, map[string]interface{}{"ok": ok, "icons": iconInfo()})
	case strings.HasPrefix(pathname, "/api/icon/") && method == "GET":
		id := strings.TrimPrefix(pathname, "/api/icon/")
		b, ct, err := ensureIcon(id)
		if err != nil {
			writeJSON(w, 404, map[string]interface{}{"ok": false, "error": "icon unavailable"})
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		_, _ = w.Write(b)
	default:
		writeJSON(w, 404, map[string]interface{}{"ok": false, "error": "unknown api"})
	}
}

func contentDisposition(filename string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, filename)
	return fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", ascii, url.PathEscape(filename))
}
