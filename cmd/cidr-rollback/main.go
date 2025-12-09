package main

import (
	"fmt"
	"ip-api/internal/logger"
	"ip-api/internal/utils"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// 文档注释：CIDR 特例段版本回滚与保留窗口
// 背景：以 source_tag 前缀分组，保留最近 N 个版本，其他版本置为 inactive；支持一键回滚。
// 约束：仅作用于 _ip_cidr_special；Exact 表的历史回滚需依赖快照或单独机制，不在此 CLI 处理。
func main() {
	_ = godotenv.Load(".env")
	l := logger.Setup()
	prefix := os.Getenv("CIDR_SOURCE_PREFIX")
	if prefix == "" {
		l.Error("cidr_prefix_missing")
		os.Exit(1)
	}
	keepN := 10
	if s := os.Getenv("CIDR_KEEP_N"); s != "" {
		var n int
		_, _ = fmt.Sscanf(s, "%d", &n)
		if n > 0 {
			keepN = n
		}
	}
	db, err := utils.OpenPostgresFromEnv()
	if err != nil {
		l.Error("db_open_error", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	q := `WITH versions AS (
            SELECT DISTINCT source_tag, max(updated_at) AS last
            FROM _ip_cidr_special WHERE source_tag LIKE $1 || '%'
            GROUP BY source_tag
          ), ranked AS (
            SELECT source_tag, last, ROW_NUMBER() OVER(ORDER BY last DESC) AS rn
            FROM versions
          )
          UPDATE _ip_cidr_special s
          SET active = CASE WHEN r.rn <= $2 THEN true ELSE false END,
              updated_at = now()
          FROM ranked r
          WHERE s.source_tag = r.source_tag AND s.source_tag LIKE $1 || '%'`
	if _, err := db.Exec(q, prefix, keepN); err != nil {
		l.Error("cidr_rollback_error", "err", err)
		os.Exit(1)
	}
	l.Info("cidr_rollback_done", "prefix", prefix, "keep", keepN)
}
