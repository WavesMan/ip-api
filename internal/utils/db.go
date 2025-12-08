// 包 utils：集中提供外部依赖初始化工具（数据库/缓存），统一环境变量读取与连接池约束
package utils

import (
    "database/sql"
    "ip-api/internal/logger"
    "os"
    "strconv"

    _ "github.com/lib/pq"
)

// OpenPostgres：使用 DSN 打开数据库连接
// 为什么：保留直接传入 DSN 的能力，便于注入与测试用例覆盖
// 约束：连接池默认上限（50/25），如需覆盖请使用 OpenPostgresFromEnv
func OpenPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	return db, nil
}

// BuildPostgresDSNFromEnv：从多行环境变量构建 Postgres DSN
// 背景：避免单行 URL 配置不可读/不可分发，支持分布式注入 PG_HOST/PORT/USER/PASSWORD/DB/SSLMODE
// 返回：形如 postgres://user:pass@host:port/db?sslmode=disable 的 DSN
func BuildPostgresDSNFromEnv() string {
  host := os.Getenv("PG_HOST")
  if host == "" { host = "127.0.0.1" }
  port := os.Getenv("PG_PORT")
  if port == "" {
    port = "5432"
  }
  user := os.Getenv("PG_USER")
  if user == "" {
    user = "postgres"
  }
  pass := os.Getenv("PG_PASSWORD")
  db := os.Getenv("PG_DB")
  if db == "" {
    db = "ipapi"
  }
  ssl := os.Getenv("PG_SSLMODE")
  if ssl == "" {
    ssl = "disable"
  }
  logger.L().Debug("pg_env", "host", host, "port", port, "user", user, "db", db, "sslmode", ssl)
  dsn := "postgres://" + user
  if pass != "" {
    dsn += ":" + pass
  }
  dsn += "@" + host + ":" + port + "/" + db + "?sslmode=" + ssl
  return dsn
}

// OpenPostgresFromEnv：读取环境变量初始化连接并设置连接池上限
// 约束：PG_MAX_OPEN_CONNS/PG_MAX_IDLE_CONNS 用于覆盖默认上限，解析失败时忽略且使用默认
func OpenPostgresFromEnv() (*sql.DB, error) {
  dsn := BuildPostgresDSNFromEnv()
  db, err := sql.Open("postgres", dsn)
  if err != nil {
    return nil, err
  }
  maxOpen := 50
  maxIdle := 25
  if v := os.Getenv("PG_MAX_OPEN_CONNS"); v != "" {
    if n, e := strconv.Atoi(v); e == nil {
      maxOpen = n
    }
  }
  if v := os.Getenv("PG_MAX_IDLE_CONNS"); v != "" {
    if n, e := strconv.Atoi(v); e == nil {
      maxIdle = n
    }
  }
  db.SetMaxOpenConns(maxOpen)
  db.SetMaxIdleConns(maxIdle)
  logger.L().Debug("pg_pool", "max_open", maxOpen, "max_idle", maxIdle)
  return db, nil
}
