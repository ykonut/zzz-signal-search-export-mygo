//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

var driveCsCache []string

func wineDriveCs() []string {
	if driveCsCache != nil {
		return driveCsCache
	}
	home, _ := os.UserHomeDir()
	roots := []string{}
	if runtime.GOOS == "darwin" {
		crossover := filepath.Join(home, "Library/Application Support/CrossOver/Bottles")
		whisky := filepath.Join(home, "Library/Containers/com.isaacmarovitz.Whisky/Data/Library/Application Support/Whisky/Bottles")
		for _, root := range []string{crossover, whisky} {
			if !isDir(root) {
				continue
			}
			if entries, err := os.ReadDir(root); err == nil {
				for _, e := range entries {
					dc := filepath.Join(root, e.Name(), "drive_c")
					if isDir(dc) {
						roots = append(roots, dc)
					}
				}
			}
		}
		support := filepath.Join(home, "Library/Application Support")
		if entries, err := os.ReadDir(support); err == nil {
			for _, e := range entries {
				if !strings.HasPrefix(strings.ToLower(e.Name()), "yaagl") {
					continue
				}
				dc := filepath.Join(support, e.Name(), "wineprefix", "drive_c")
				if isDir(dc) {
					roots = append(roots, dc)
				}
			}
		}
	}
	if runtime.GOOS == "linux" {
		roots = append(roots, filepath.Join(home, ".wine", "drive_c"))
		// Steam / Proton prefixes
		for _, base := range []string{
			filepath.Join(home, ".local/share/Steam/steamapps/compatdata"),
			filepath.Join(home, ".steam/steam/steamapps/compatdata"),
		} {
			if entries, err := os.ReadDir(base); err == nil {
				for _, e := range entries {
					dc := filepath.Join(base, e.Name(), "pfx", "drive_c")
					if isDir(dc) {
						roots = append(roots, dc)
					}
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, r := range roots {
		if !seen[r] {
			seen[r] = true
			driveCsCache = append(driveCsCache, r)
		}
	}
	return driveCsCache
}

func winePathToMac(p string) string {
	s := strings.TrimSpace(strings.ReplaceAll(p, `\\`, `\`))
	m := regexp.MustCompile(`^([A-Za-z]):[\\/](.*)$`).FindStringSubmatch(s)
	if m == nil {
		if strings.HasPrefix(s, "/") {
			return s
		}
		return ""
	}
	rest := strings.ReplaceAll(m[2], `\`, "/")
	if strings.ToUpper(m[1]) == "Z" {
		return "/" + rest
	}
	for _, dc := range wineDriveCs() {
		cand := filepath.Join(dc, rest)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return ""
}

func normalizeGameDir(input string) string {
	if input == "" {
		return ""
	}
	s := strings.Trim(strings.TrimSpace(input), `"'`)
	mac := winePathToMac(s)
	if mac == "" {
		mac = s
	}
	if strings.HasSuffix(strings.ToLower(mac), "zenlesszonezero.exe") {
		mac = filepath.Dir(mac)
	}
	for _, c := range []string{mac, filepath.Join(mac, "ZenlessZoneZero_Data"), filepath.Join(mac, "..")} {
		if isDir(filepath.Join(c, "webCaches")) {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func findGameDirsFromProcesses() []string {
	out := map[string]bool{}
	for _, pattern := range []string{"ZenlessZoneZero", "ZFGameBrowser"} {
		cmd := exec.Command("pgrep", "-fl", pattern)
		b, err := cmd.Output()
		if err != nil {
			continue
		}
		re := regexp.MustCompile(`([A-Za-z]:[\\/][^\s"']*?)(?:ZenlessZoneZero\.exe|ZenlessZoneZero_Data)`)
		for _, line := range strings.Split(string(b), "\n") {
			if m := re.FindStringSubmatch(line); len(m) > 1 {
				if d := normalizeGameDir(m[1]); d != "" {
					out[d] = true
				}
			}
		}
	}
	return keys(out)
}

func keys(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

func findGameDirsInBottle(driveC string) []string {
	results := []string{}
	visited := 0
	skip := map[string]bool{"windows": true, "programdata": true, "$recycle.bin": true, "system volume information": true}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if len(results) > 0 || visited > 4000 || depth > 6 {
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
			if skip[strings.ToLower(e.Name())] {
				continue
			}
			visited++
			p := filepath.Join(dir, e.Name())
			if e.Name() == "ZenlessZoneZero_Data" {
				if isDir(filepath.Join(p, "webCaches")) {
					results = append(results, p)
					return
				}
				continue
			}
			walk(p, depth+1)
			if len(results) > 0 {
				return
			}
		}
	}
	walk(driveC, 0)
	return results
}

func findGameDirsFromBottles() []string {
	out := map[string]bool{}
	for _, dc := range wineDriveCs() {
		for _, d := range findGameDirsInBottle(dc) {
			out[d] = true
		}
	}
	return keys(out)
}

func findGameDirsFromVolumes() []string {
	out := map[string]bool{}
	roots := []string{"/Volumes"}
	if runtime.GOOS == "linux" {
		home, _ := os.UserHomeDir()
		roots = append(roots,
			"/mnt", "/media", "/run/media",
			filepath.Join(home, ".local/share/Steam/steamapps/common"),
			filepath.Join(home, ".steam/steam/steamapps/common"),
			filepath.Join(home, "Games"),
		)
	}
	for _, base := range roots {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			root := filepath.Join(base, e.Name())
			if !isDir(root) {
				continue
			}
			cands := []string{root}
			if subs, err := os.ReadDir(root); err == nil {
				for _, s := range subs {
					p := filepath.Join(root, s.Name())
					if isDir(p) {
						cands = append(cands, p)
					}
				}
			}
			for _, p := range cands {
				if isDir(filepath.Join(p, "ZenlessZoneZero_Data", "webCaches")) {
					out[p] = true
					out[filepath.Join(p, "ZenlessZoneZero_Data")] = true
				}
				if isDir(filepath.Join(p, "webCaches")) {
					out[p] = true
				}
			}
		}
	}
	return keys(out)
}

func findGachaURL() *WineHit {
	dirs := map[string]bool{}
	for _, d := range findGameDirsFromProcesses() {
		dirs[d] = true
	}
	if len(dirs) == 0 {
		for _, d := range findGameDirsFromBottles() {
			dirs[d] = true
		}
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
