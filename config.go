package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	Lang             string      `json:"lang"`
	Current          interface{} `json:"current"`
	FetchFullHistory bool        `json:"fetchFullHistory"`
}

var (
	configMu sync.RWMutex
	config   = Config{Lang: "zh-cn", Current: "0"}
)

func configPath() string { return filepath.Join(dataDir, "config.json") }

func loadConfig() {
	b, err := os.ReadFile(configPath())
	if err != nil {
		return
	}
	var local map[string]json.RawMessage
	if json.Unmarshal(b, &local) != nil {
		return
	}
	configMu.Lock()
	defer configMu.Unlock()
	if v, ok := local["lang"]; ok {
		_ = json.Unmarshal(v, &config.Lang)
	}
	if v, ok := local["current"]; ok {
		_ = json.Unmarshal(v, &config.Current)
	}
	if v, ok := local["fetchFullHistory"]; ok {
		_ = json.Unmarshal(v, &config.FetchFullHistory)
	}
	if config.Lang != "zh-cn" && config.Lang != "zh-tw" && config.Lang != "en-us" {
		config.Lang = "zh-cn"
	}
}

func getConfig() Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return config
}

func currentUID() string {
	configMu.RLock()
	defer configMu.RUnlock()
	return anyToString(config.Current)
}

func saveConfig() error {
	configMu.RLock()
	b, err := json.MarshalIndent(config, "", "  ")
	configMu.RUnlock()
	if err != nil {
		return err
	}
	tmp := configPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, configPath())
}

func setConfigValue(key string, value interface{}) bool {
	configMu.Lock()
	defer configMu.Unlock()
	switch key {
	case "lang":
		s, _ := value.(string)
		if s != "zh-cn" && s != "zh-tw" && s != "en-us" {
			return false
		}
		config.Lang = s
	case "current":
		config.Current = anyToString(value)
	case "fetchFullHistory":
		b, ok := value.(bool)
		if !ok {
			return false
		}
		config.FetchFullHistory = b
	default:
		return false
	}
	return true
}

func configForJSON() map[string]interface{} {
	c := getConfig()
	return map[string]interface{}{
		"lang": c.Lang, "current": c.Current, "fetchFullHistory": c.FetchFullHistory,
	}
}
