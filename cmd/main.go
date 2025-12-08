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
	importIP := os.Getenv("IMPORT_IPIP_TO_DB") == "true"
	if importIP {
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
		l.Info("ipip_import_skipped", "reason", "sql_no_cidr_default")
	}

	// 数据库健康度自检与自愈（表被删除或为空时自动重建并导入特例段）
	go func() {
		autoRepair := os.Getenv("AUTO_REPAIR_DB")
		if autoRepair == "" || autoRepair == "true" {
			var locCount, specialCount int64
			_ = db.QueryRow("SELECT COUNT(1) FROM _ip_locations").Scan(&locCount)
			_ = db.QueryRow("SELECT COUNT(1) FROM _ip_cidr_special").Scan(&specialCount)
			if (locCount == 0 || specialCount == 0) && ipipPath != "" {
				if r, err := ipip.Open(ipipPath); err == nil {
					lang := os.Getenv("IPIP_LANG")
					if lang == "" {
						lang = "zh-CN"
					}
					l.Info("ipip_special_import_begin", "lang", lang)
					if err := ipip.ImportIPv4LeavesToSpecial(db, r, lang, "ipip"); err != nil {
						l.Error("ipip_special_import_error", "err", err)
					} else {
						l.Info("ipip_special_import_success")
					}
				} else {
					l.Error("ipip_open_error", "err", err)
				}
			}
		}
	}()

	mux := http.NewServeMux()
	// 背景：读取已下载的 mmdb 构建查询服务；失败不影响静态文件与数据库导入部分
	// 背景：构建本地压缩内存缓存用于快速随机读取；支持后续重建
	// 动态文件分片缓存：在导入后就绪时构建并加载，避免空库时无分片
	fileDir := filepath.Join("data", "localdb")
	l.Debug("config_localdb_dir", "dir", fileDir)
	var dcache localdb.DynamicCache
	go func() {
		for {
			var haveOverrides int64
			_ = db.QueryRow("SELECT COUNT(1) FROM _ip_overrides").Scan(&haveOverrides)
			var mc interface {
				Lookup(string) (localdb.Location, bool)
			}
			var exact interface {
				Lookup(string) (localdb.Location, bool)
			}
			var iptree interface {
				Lookup(string) (localdb.Location, bool)
			}
			if haveOverrides > 0 {
				if err := localdb.BuildExactDBFromDB(fileDir, db); err == nil {
					if edb, err := localdb.NewExactDB(fileDir, db); err == nil {
						exact = edb
						l.Info("exactdb_ready")
					}
				} else {
					l.Error("exactdb_build_error", "err", err)
				}
			} else {
				l.Debug("exactdb_skip", "reason", "no_overrides")
			}
			lang := os.Getenv("IPIP_LANG")
			if lang == "" {
				lang = "zh-CN"
			}
			if iptree2, err := localdb.NewIPIPCache(ipipPath, lang); err == nil {
				iptree = iptree2
				l.Info("ipiptree_ready")
			} else {
				l.Error("ipiptree_error", "err", err)
			}
			if exact != nil || iptree != nil {
				mc = localdb.NewMultiCache(exact, iptree)
				dcache.Set(mc)
				l.Info("filecache_ready")
				l.Debug("cache_stack", "exact", exact != nil, "tree", iptree != nil)
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
