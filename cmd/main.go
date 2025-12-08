// 程序入口：仅负责读取配置、初始化依赖并启动服务；API 注册在 internal/api 以便扩展
package main

import (
	"context"
	"ip-api/internal/api"
	"ip-api/internal/ipip"
	"ip-api/internal/localdb"
	"ip-api/internal/logger"
	"ip-api/internal/migrate"
	"ip-api/internal/store"
	"ip-api/internal/utils"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")
	// 日志初始化
	l := logger.Setup()
	l.Debug("log_init_ok")
	apiBase := os.Getenv("API_BASE")
	if apiBase == "" {
		apiBase = "/api"
	}
	l.Debug("config_api_base", "base", apiBase)
	_ = utils.BuildPostgresDSNFromEnv()
	ui := os.Getenv("UI_DIST")
	if ui == "" {
		ui = filepath.Join("ui", "dist")
	}
	l.Debug("config_ui_dir", "dir", ui)

	db, err := utils.OpenPostgresFromEnv()
	if err != nil {
		l.Error("db_open_error", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	l.Info("db_open_ok")
	if err := db.Ping(); err != nil {
		l.Error("db_ping_error", "err", err)
	} else {
		l.Info("db_ping_ok")
	}
	st := store.AttachDB(db)
	if err := migrate.EnsureSchema(db); err != nil {
		l.Error("schema_error", "err", err)
		os.Exit(1)
	}

	rc := utils.OpenRedisFromEnv()
	if rc == nil {
		l.Info("redis_disabled")
	} else {
		if err := rc.Ping(context.Background()).Err(); err != nil {
			l.Error("redis_ping_error", "err", err)
		} else {
			l.Info("redis_ping_ok")
		}
	}

	// 背景：已废弃远程数据源导入；仅使用本地 IPIP 初始化与写库

	// 背景：废弃自动更新，改用本地 ipip 数据源；启动时并行导入到数据库
	ipipPath := os.Getenv("IPIP_PATH")
	if ipipPath == "" {
		ipipPath = filepath.Join("data", "ipip", "ipipfree.ipdb")
	}
	l.Debug("config_ipip_path", "path", ipipPath)
	var rngCount int64
	_ = db.QueryRow("SELECT COUNT(1) FROM _ip_ipv4_ranges").Scan(&rngCount)
	l.Debug("ranges_count_before_import", "count", rngCount)
	if rngCount == 0 {
		if _, err := os.Stat(ipipPath); err == nil {
			l.Info("ipip_found", "path", ipipPath)
			if r, err := ipip.Open(ipipPath); err == nil {
				lang := os.Getenv("IPIP_LANG")
				if lang == "" {
					lang = "zh-CN"
				}
				l.Info("ipip_import_begin", "lang", lang)
				go func() {
					if err := ipip.ImportIPv4LeavesToDBConcurrent(db, r, lang); err != nil {
						l.Error("ipip_import_error", "err", err)
					} else {
						l.Info("ipip_import_success")
					}
				}()
			} else {
				l.Error("ipip_open_error", "err", err)
			}
		} else {
			l.Error("ipip_not_found", "path", ipipPath)
		}
	} else {
		l.Info("ipip_import_skipped", "reason", "ranges_not_empty")
	}

	mux := http.NewServeMux()
	// 背景：读取已下载的 mmdb 构建查询服务；失败不影响静态文件与数据库导入部分
	// 背景：构建本地压缩内存缓存用于快速随机读取；支持后续重建
	// 动态文件分片缓存：在导入后就绪时构建并加载，避免空库时无分片
	fileDir := filepath.Join("data", "localdb")
	l.Debug("config_localdb_dir", "dir", fileDir)
	var dcache localdb.DynamicCache
	go func() {
		for {
			var c int64
			row := db.QueryRow("SELECT COUNT(1) FROM _ip_ipv4_ranges")
			_ = row.Scan(&c)
			if c > 0 {
				if err := localdb.BuildFilesFromDB(fileDir, db); err != nil {
					l.Error("filecache_build_error", "err", err)
					time.Sleep(2 * time.Second)
					continue
				}
				if fc, err := localdb.NewFileCache(fileDir); err == nil {
					dcache.Set(fc)
					l.Info("filecache_ready")
				} else {
					l.Error("filecache_init_error", "err", err)
					time.Sleep(2 * time.Second)
					continue
				}
				break
			}
			time.Sleep(2 * time.Second)
		}
	}()
	apiMux := api.BuildRoutes(st, rc, &dcache)
	mux.Handle(apiBase+"/", http.StripPrefix(apiBase, apiMux))

	fs := http.FileServer(http.Dir(ui))
	mux.Handle("/", fs)

	// NOTE: 向前端暴露 API 基础路径，避免硬编码；生产环境由后端统一提供
	mux.HandleFunc("/config.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/javascript; charset=utf-8")
		w.Header().Set("cache-control", "no-store")
		_, _ = w.Write([]byte("window.__API_BASE__='" + apiBase + "'"))
		_, _ = w.Write([]byte("\n"))
		_, _ = w.Write([]byte("window.__DATA_SOURCE__='IPIP 数据库'"))
		_, _ = w.Write([]byte("\n"))
		_, _ = w.Write([]byte("window.__DATA_SOURCE_URL__='https://www.ipip.net'"))
	})

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	handler := logger.AccessMiddleware(l)(mux)
	s := &http.Server{Addr: addr, Handler: handler}
	l.Info("listening", "addr", addr)
	_ = s.ListenAndServe()
}
