package main

import (
	"embed"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
)

//go:embed web resources
var embedded embed.FS

var dataDir string

func main() {
	home, _ := os.UserHomeDir()
	dataDir = filepath.Join(home, "Library", "Application Support", "ZZZ-Signal-Search-Export")
	if runtime.GOOS == "windows" {
		if d, err := os.UserConfigDir(); err == nil {
			dataDir = filepath.Join(d, "ZZZ-Signal-Search-Export")
		}
	} else if runtime.GOOS == "linux" {
		if d, err := os.UserConfigDir(); err == nil {
			dataDir = filepath.Join(d, "zzz-signal-search-export")
		}
	}
	if v := os.Getenv("ZZZ_DATA_DIR"); v != "" {
		dataDir = v
	}
	_ = os.MkdirAll(dataDir, 0755)
	logPath := filepath.Join(dataDir, "app.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
		defer logFile.Close()
	}
	log.Printf("app launching (MyGo build) data=%s", dataDir)

	loadConfig()
	if err := loadI18n(); err != nil {
		log.Printf("i18n load failed: %v", err)
	}
	if err := initResources(); err != nil {
		log.Printf("resources init failed: %v", err)
	}
	if err := loadIcons(); err != nil {
		log.Printf("icons init failed: %v", err)
	}
	readData()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	go func() {
		if err := http.Serve(ln, newMux()); err != nil {
			log.Printf("http server stopped: %v", err)
		}
	}()
	log.Printf("local server on http://127.0.0.1:%d", port)

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "绝区零调频记录导出",
			URL:       fmt.Sprintf("http://127.0.0.1:%d/", port),
			Width:     1120,
			Height:    780,
			MinWidth:  820,
			MinHeight: 560,
		})
		if snapshot := os.Getenv("MYGO_SNAPSHOT"); snapshot != "" {
			go func() {
				time.Sleep(3 * time.Second)
				if b, err := win.CapturePage(); err == nil {
					if err := os.WriteFile(snapshot, b, 0644); err == nil {
						log.Printf("snapshot saved: %s", snapshot)
					}
				} else {
					log.Printf("snapshot failed: %v", err)
				}
			}()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
