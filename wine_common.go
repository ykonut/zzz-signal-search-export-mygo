package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type WineHit struct {
	URL    string
	Source string
	File   string
	Mtime  int64
}

func isDir(p string) bool { st, err := os.Stat(p); return err == nil && st.IsDir() }

func pickGachaURL(data []byte) string {
	best := ""
	idx := 0
	for {
		i := bytes.Index(data[idx:], []byte("https://"))
		if i < 0 {
			break
		}
		i += idx
		j := i
		for j < len(data) && data[j] >= 0x21 && data[j] <= 0x7e {
			j++
		}
		u := string(data[i:j])
		if strings.Contains(u, "getGachaLog") && regexp.MustCompile(`[?&]authkey=[^&]+`).MatchString(u) {
			if len(u) > len(best) {
				best = u
			}
		}
		idx = j
	}
	return best
}

func readGachaURLFromDir(gameDir string) *WineHit {
	wc := filepath.Join(gameDir, "webCaches")
	if !isDir(wc) {
		wc = filepath.Join(gameDir, "ZenlessZoneZero_Data", "webCaches")
	}
	versions, err := os.ReadDir(wc)
	if err != nil {
		return nil
	}
	type fileInfo struct {
		path  string
		mtime int64
	}
	files := []fileInfo{}
	for _, v := range versions {
		if !v.IsDir() {
			continue
		}
		cacheDir := filepath.Join(wc, v.Name(), "Cache", "Cache_Data")
		for _, name := range []string{"data_2", "data_1", "data_0", "data_3"} {
			fp := filepath.Join(cacheDir, name)
			if st, err := os.Stat(fp); err == nil && st.Mode().IsRegular() && st.Size() <= 32*1024*1024 {
				files = append(files, fileInfo{fp, st.ModTime().UnixMilli()})
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mtime > files[j].mtime })
	for _, f := range files {
		b, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		if u := pickGachaURL(b); u != "" {
			return &WineHit{URL: u, Source: "CEF 缓存", File: f.path, Mtime: f.mtime}
		}
	}
	return nil
}
