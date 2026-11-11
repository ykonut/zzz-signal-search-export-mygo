//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func normalizeGameDir(input string) string {
	if input == "" {
		return ""
	}
	s := strings.Trim(strings.TrimSpace(input), `"'`)
	if strings.HasSuffix(strings.ToLower(s), "zenlesszonezero.exe") {
		s = filepath.Dir(s)
	}
	for _, c := range []string{s, filepath.Join(s, "ZenlessZoneZero_Data"), filepath.Join(s, "..")} {
		if isDir(filepath.Join(c, "webCaches")) {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func findGameDirsFromProcesses() []string {
	out := map[string]bool{}
	script := `Get-Process ZenlessZoneZero,ZFGameBrowser -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Path`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	b, err := cmd.Output()
	if err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if d := normalizeGameDir(strings.TrimSpace(line)); d != "" {
				out[d] = true
			}
		}
	}
	return mapKeys(out)
}

func mapKeys(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

func findGameDirsFromVolumes() []string {
	out := map[string]bool{}
	roots := []string{}
	for _, e := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA", "USERPROFILE"} {
		if p := os.Getenv(e); p != "" {
			roots = append(roots, p)
		}
	}
	// 常见盘符根目录
	for c := 'C'; c <= 'Z'; c++ {
		root := string(c) + `:\`
		if isDir(root) {
			roots = append(roots, root)
		}
	}
	seenRoot := map[string]bool{}
	visited := 0
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if len(out) > 0 || depth > 6 || visited > 6000 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := strings.ToLower(e.Name())
			if name == "windows" || name == "programdata" || name == "$recycle.bin" || name == "system volume information" {
				continue
			}
			visited++
			p := filepath.Join(dir, e.Name())
			if e.Name() == "ZenlessZoneZero_Data" {
				if isDir(filepath.Join(p, "webCaches")) {
					out[p] = true
					return
				}
				continue
			}
			walk(p, depth+1)
			if len(out) > 0 {
				return
			}
		}
	}
	for _, root := range roots {
		if root == "" || seenRoot[root] {
			continue
		}
		seenRoot[root] = true
		walk(root, 0)
		if len(out) > 0 {
			break
		}
	}
	outList := mapKeys(out)
	for i := range outList {
		if strings.HasSuffix(strings.ToLower(outList[i]), "zenlesszonezero_data") {
			outList[i] = outList[i]
		}
	}
	return outList
}

func findGachaURL() *WineHit {
	dirs := map[string]bool{}
	for _, d := range findGameDirsFromProcesses() {
		dirs[d] = true
	}
	if len(dirs) == 0 {
		for _, d := range findGameDirsFromVolumes() {
			dirs[d] = true
		}
	}
	var hits []*WineHit
	for d := range dirs {
		if h := readGachaURLFromDir(d); h != nil {
			hits = append(hits, h)
		}
	}
	if len(hits) == 0 {
		return nil
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Mtime > hits[j].Mtime })
	return hits[0]
}
