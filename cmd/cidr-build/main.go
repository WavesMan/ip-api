package main

import (
	"context"
	"fmt"
	"ip-api/internal/logger"
	"ip-api/internal/utils"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// 文档注释：从 _ip_exact 压缩构建 CIDR 特例段
// 背景：将同一 source_tag、相同 location 的连续 IP 聚合为段，落入 _ip_cidr_special，纳入版本管理与回滚体系。
// 约束：最小段长度可配置（默认 8 个连续 IP）；只聚合同一 location_id；只处理 IPv4。
func main() {
	_ = godotenv.Load(".env")
	l := logger.Setup()
	tag := os.Getenv("CIDR_SOURCE_TAG")
	if tag == "" {
		l.Error("cidr_source_tag_missing")
		os.Exit(1)
	}
	minLen := 8
	if s := os.Getenv("CIDR_MIN_LEN"); s != "" {
		var n int
		_, _ = fmt.Sscanf(s, "%d", &n)
		if n > 1 {
			minLen = n
		}
	}
	db, err := utils.OpenPostgresFromEnv()
	if err != nil {
		l.Error("db_open_error", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT ip_int, location_id FROM _ip_exact WHERE source_tag LIKE $1 || '%' ORDER BY location_id, ip_int`, tag)
	if err != nil {
		l.Error("exact_scan_error", "err", err)
		os.Exit(1)
	}
	defer rows.Close()
	type item struct {
		ip  int64
		loc int
	}
	var cur []item
	var lastLoc int
	var lastIP int64
	flush := func() {
		if len(cur) >= minLen {
			start := cur[0].ip
			end := cur[len(cur)-1].ip
			a := int((start >> 24) & 0xff)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err := db.ExecContext(ctx, `INSERT INTO _ip_cidr_special(start_int,end_int,first_octet,location_id,source_tag,active) VALUES($1,$2,$3,$4,$5,TRUE)`, start, end, a, lastLoc, tag+"-cidr")
			cancel()
			if err != nil {
				l.Error("cidr_insert_error", "err", err)
			} else {
				l.Debug("cidr_insert", "start", start, "end", end, "loc", lastLoc)
			}
		}
		cur = cur[:0]
	}
	for rows.Next() {
		var ipInt int64
		var locID int
		if err := rows.Scan(&ipInt, &locID); err != nil {
			l.Error("exact_scan_item_error", "err", err)
			os.Exit(1)
		}
		if len(cur) == 0 {
			cur = append(cur, item{ip: ipInt, loc: locID})
			lastLoc = locID
			lastIP = ipInt
			continue
		}
		if locID != lastLoc || ipInt != lastIP+1 {
			flush()
			cur = append(cur, item{ip: ipInt, loc: locID})
			lastLoc = locID
			lastIP = ipInt
			continue
		}
		cur = append(cur, item{ip: ipInt, loc: locID})
		lastIP = ipInt
	}
	flush()
	l.Info("cidr_build_done")
}
