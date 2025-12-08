// 程序入口：仅负责读取配置、初始化依赖并启动服务；API 注册在 internal/api 以便扩展
package main

import (
	"ip-api/internal/api"
	"ip-api/internal/logger"
	"ip-api/internal/store"
	"ip-api/internal/utils"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	l := logger.Setup()
	// 从环境变量构建 DSN（实际由 OpenPostgresFromEnv 内部使用，无需显式变量）
	_ = utils.BuildPostgresDSNFromEnv()
	ui := os.Getenv("UI_DIST")
	if ui == "" {
		ui = filepath.Join("ui", "dist")
	}

	db, err := utils.OpenPostgresFromEnv()
	if err != nil {
		l.Error("db_open_error", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	st := store.AttachDB(db)

	rc := utils.OpenRedisFromEnv()

	mux := http.NewServeMux()
	apiMux := api.BuildRoutes(st, rc)
	mux.Handle("/api/", http.StripPrefix("/api", apiMux))

	fs := http.FileServer(http.Dir(ui))
	mux.Handle("/", fs)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	handler := logger.AccessMiddleware(l)(mux)
	s := &http.Server{Addr: addr, Handler: handler}
	l.Info("listening", "addr", addr)
	_ = s.ListenAndServe()
}
