// 程序入口：仅负责读取配置、初始化依赖并启动服务；API 注册在 internal/api 以便扩展
package main

import (
	"context"
	"ip-api/internal/api"
	"ip-api/internal/fusion"
	"ip-api/internal/ipip"
	"ip-api/internal/localdb"
	"ip-api/internal/localdb/chain"
	"ip-api/internal/localdb/exact"
	"ip-api/internal/localdb/ip2region"
	ipipcache "ip-api/internal/localdb/ipip"
	"ip-api/internal/logger"
	"ip-api/internal/metrics"
	"ip-api/internal/middleware"
	"ip-api/internal/migrate"
	"ip-api/internal/plugins"
	"ip-api/internal/store"
	"ip-api/internal/utils"
	"ip-api/internal/version"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")
	_ = godotenv.Load(filepath.Join("data", "env", ".env"))
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
	// 文档注释：插件管理器初始化
	// 背景：统一管理内置/外部插件，提供健康插件集合给融合层；在后台启动心跳监控。
	pm := plugins.NewManager()
	pm.Register(plugins.NewBuiltin("kv", "1.0", "kv", &fusion.KVSource{Store: st}))
	l.Info("plugin_register", "name", "kv")
	// 文档注释：注册 AMap 内置插件（在线查询）
	// 背景：作为实时数据源参与融合；权重来自环境变量；需要服务端密钥。
	if key := os.Getenv("AMAP_SERVER_KEY"); key != "" {
		client := &http.Client{Timeout: 4 * time.Second}
		pm.Register(plugins.NewAMapPlugin(key, client))
		l.Info("plugin_register", "name", "amap")
	}
	pm.Start(context.Background())
	go func() {
		for {
			var haveOverrides int64
			_ = db.QueryRow("SELECT COUNT(1) FROM _ip_overrides").Scan(&haveOverrides)
			var haveOverridesKV int64
			_ = db.QueryRow("SELECT COUNT(1) FROM _ip_overrides_kv").Scan(&haveOverridesKV)
			var mc interface {
				Lookup(string) (localdb.Location, bool)
			}
			var exactCache interface {
				Lookup(string) (localdb.Location, bool)
			}
			var iptree interface {
				Lookup(string) (localdb.Location, bool)
			}
			var ip2r interface {
				Lookup(string) (localdb.Location, bool)
			}
			if haveOverrides > 0 || haveOverridesKV > 0 {
				if err := exact.BuildExactDBFromDB(fileDir, db); err == nil {
					if edb, err := exact.NewExactDB(fileDir, db); err == nil {
						exactCache = edb
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
			if iptree2, err := ipipcache.NewIPIPCache(ipipPath, lang); err == nil {
				iptree = iptree2
				l.Info("ipiptree_ready")
			} else {
				l.Error("ipiptree_error", "err", err)
			}
			// IP2Region v4（按需）
			ip2rV4 := os.Getenv("IP2REGION_V4_PATH")
			if ip2rV4 != "" {
				if c, err := ip2region.NewIP2RegionCache(ip2rV4, ""); err == nil {
					ip2r = c
					l.Info("ip2region_ready")
				} else {
					l.Error("ip2region_error", "err", err)
				}
			}
			if exactCache != nil || iptree != nil || ip2r != nil {
				mc = chain.NewChainCache(exactCache, iptree, ip2r)
				dcache.Set(mc)
				l.Info("filecache_ready")
				l.Debug("cache_stack", "exact", exactCache != nil, "tree", iptree != nil, "ip2r", ip2r != nil)
				// 文档注释：注册内置插件（依赖缓存就绪）
				// 背景：IPIP/IP2Region 作为内置插件加入融合；权重由环境变量或默认值决定。
				if iptree != nil {
					pm.Register(plugins.NewBuiltin("ipip", "1.0", "ipip", &fusion.IPIPSource{Cache: iptree}))
					l.Info("plugin_register", "name", "ipip")
				}
				if ip2r != nil {
					pm.Register(plugins.NewIP2RegionPlugin(ip2r))
					l.Info("plugin_register", "name", "ip2region")
				}
				break
			}
			time.Sleep(2 * time.Second)
		}
	}()
	// 文档注释：可选注册外部 HTTP 插件
	// 背景：通过简单 HTTP 契约接入第三方数据源；避免 Go 动态插件在 Windows 的可移植性问题。
	if ep := os.Getenv("EXT_PLUGIN_ENDPOINT"); ep != "" {
		name := os.Getenv("EXT_PLUGIN_NAME")
		if name == "" {
			name = "ext"
		}
		assoc := os.Getenv("EXT_PLUGIN_ASSOC")
		if assoc == "" {
			assoc = "ext"
		}
		w := 5.0
		if s := os.Getenv("EXT_PLUGIN_WEIGHT"); s != "" {
			if n, e := strconv.ParseFloat(s, 64); e == nil && n > 0 {
				w = n
			}
		}
		pm.Register(plugins.NewHTTP(name, "1.0", assoc, ep, w))
		l.Info("plugin_register", "name", name, "assoc", assoc)
	}
	// 文档注释：构建路由（携带动态缓存与插件管理器）
	apiMux := api.BuildRoutes(st, rc, &dcache, pm)
	mux.Handle(apiBase+"/", http.StripPrefix(apiBase, apiMux))
	mux.Handle(apiBase+"/metrics", metrics.Handler())
	mux.HandleFunc(apiBase+"/reload-exact", func(w http.ResponseWriter, r *http.Request) {
		t := r.Header.Get("x-admin-token")
		if t == "" || t != os.Getenv("ADMIN_TOKEN") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		lang := os.Getenv("IPIP_LANG")
		if lang == "" {
			lang = "zh-CN"
		}
		var exactCache interface {
			Lookup(string) (localdb.Location, bool)
		}
		if err := exact.BuildExactDBFromDB(fileDir, db); err == nil {
			if edb, err := exact.NewExactDB(fileDir, db); err == nil {
				exactCache = edb
				l.Info("exactdb_reloaded")
			} else {
				l.Error("exactdb_open_error", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		} else {
			l.Error("exactdb_build_error", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var iptree interface {
			Lookup(string) (localdb.Location, bool)
		}
		if iptree2, err := ipipcache.NewIPIPCache(ipipPath, lang); err == nil {
			iptree = iptree2
		} else {
			l.Error("ipiptree_error", "err", err)
		}
		mc := chain.NewMultiCache(exactCache, iptree)
		dcache.Set(mc)
		w.WriteHeader(http.StatusNoContent)
	})

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
		_, _ = w.Write([]byte("\n"))
		_, _ = w.Write([]byte("window.__COMMIT_SHA__='" + version.Commit + "'"))
	})

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	handler := logger.AccessMiddleware(l)(mux)
	handler = middleware.Wrap(handler)
	s := &http.Server{Addr: addr, Handler: handler}
	tlsEnable := os.Getenv("TLS_ENABLE")
	if tlsEnable == "" || tlsEnable == "true" {
		certPath := os.Getenv("TLS_CERT_PATH")
		keyPath := os.Getenv("TLS_KEY_PATH")
		if certPath == "" {
			certPath = filepath.Join("data", "certs", "server.crt")
		}
		if keyPath == "" {
			keyPath = filepath.Join("data", "certs", "server.key")
		}
		_ = utils.EnsureSelfSignedCert(certPath, keyPath, "ip-api.local")
		// 可选：启动HTTP重定向到HTTPS（不改变HTTPS运行端口）
		if os.Getenv("TLS_REDIRECT_ENABLE") == "true" {
			redirAddr := os.Getenv("TLS_REDIRECT_ADDR")
			if redirAddr == "" {
				redirAddr = ":80"
			}
			go func() {
				httpRedir := http.NewServeMux()
				httpRedir.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
					host := r.Host
					// 替换目标端口为HTTPS服务端口
					httpsPort := strings.TrimPrefix(addr, ":")
					baseHost := host
					if i := strings.LastIndex(host, ":"); i != -1 {
						baseHost = host[:i]
					}
					targetHost := baseHost
					if httpsPort != "" {
						targetHost = baseHost + ":" + httpsPort
					}
					target := "https://" + targetHost + r.URL.RequestURI()
					http.StatusText(http.StatusMovedPermanently)
					http.Redirect(w, r, target, http.StatusMovedPermanently)
					l.Debug("http_redirect", "from", r.Host, "to", target)
				})
				l.Info("http_redirect_listening", "addr", redirAddr, "to", "https"+addr)
				_ = http.ListenAndServe(redirAddr, logger.AccessMiddleware(l)(httpRedir))
			}()
		}
		l.Info("listening_tls", "addr", addr, "cert", certPath)
		_ = s.ListenAndServeTLS(certPath, keyPath)
		return
	}
	l.Info("listening", "addr", addr)
	_ = s.ListenAndServe()
}
