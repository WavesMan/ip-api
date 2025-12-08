// 包 store: 提供与 PostgreSQL 的数据访问层，包含 IP 查询与统计读写
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"ip-api/internal/logger"

	_ "github.com/lib/pq"
)

// Store: 数据库访问入口，持有连接池并提供查询/统计接口
type Store struct {
	db *sql.DB
}

func AttachDB(db *sql.DB) *Store { return &Store{db: db} }

// Location: 归属地字典结构，表示一次查询命中的地域信息
type Location struct {
	Country  string
	Region   string
	Province string
	City     string
	ISP      string
}

// Open: 使用 DSN 打开数据库连接并配置连接池参数
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	return &Store{db: db}, nil
}

// Close: 关闭数据库连接
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }

// ipToInt: 将 IPv4 文本转换为无符号整数，非法返回错误
func ipToInt(ip string) (uint32, error) {
	var a, b, c, d int
	n, err := fmt.Sscanf(ip, "%d.%d.%d.%d", &a, &b, &c, &d)
	if err != nil || n != 4 {
		return 0, errors.New("bad ip")
	}
	if a < 0 || a > 255 || b < 0 || b > 255 || c < 0 || c > 255 || d < 0 || d > 255 {
		return 0, errors.New("bad ip")
	}
	x := uint32(a)<<24 | uint32(b)<<16 | uint32(c)<<8 | uint32(d)
	return x, nil
}

// LookupIP: 查询单个 IPv4 的归属地，先按首段定位分区，再在范围内匹配
func (s *Store) LookupIP(ctx context.Context, ip string) (*Location, error) {
	val, err := ipToInt(ip)
	if err != nil {
		return nil, nil
	}
	logger.L().Debug("db_lookup_begin", "ip", ip, "val", int64(val))
	row0 := s.db.QueryRowContext(ctx, "SELECT country, region, province, city, isp FROM _ip_overrides_kv WHERE ip_int=$1 LIMIT 1", int64(val))
	var lk Location
	if err := row0.Scan(&lk.Country, &lk.Region, &lk.Province, &lk.City, &lk.ISP); err == nil {
		logger.L().Debug("db_override_kv_hit", "ip_val", int64(val))
		return &lk, nil
	}
	row := s.db.QueryRowContext(ctx, "SELECT location_id FROM _ip_overrides WHERE ip_int=$1 LIMIT 1", int64(val))
	var locID int
	if err := row.Scan(&locID); err != nil {
		row2 := s.db.QueryRowContext(ctx, "SELECT location_id FROM _ip_exact WHERE ip_int=$1 LIMIT 1", int64(val))
		if err := row2.Scan(&locID); err != nil {
			a := int((val >> 24) & 0xff)
			row3 := s.db.QueryRowContext(ctx, "SELECT location_id FROM _ip_cidr_special WHERE first_octet=$1 AND start_int<=$2 AND end_int>=$2 AND active=TRUE ORDER BY (end_int - start_int) ASC, start_int DESC LIMIT 1", a, int64(val))
			if err := row3.Scan(&locID); err != nil {
				logger.L().Debug("db_lookup_miss", "ip_val", int64(val))
				return nil, nil
			}
			logger.L().Debug("db_special_hit", "ip_val", int64(val), "loc_id", locID)
		} else {
			logger.L().Debug("db_exact_hit", "ip_val", int64(val), "loc_id", locID)
		}
	} else {
		logger.L().Debug("db_override_hit", "ip_val", int64(val), "loc_id", locID)
	}
	row2 := s.db.QueryRowContext(ctx, "SELECT country, region, province, city, isp FROM _ip_locations WHERE id=$1", locID)
	var l Location
	if err := row2.Scan(&l.Country, &l.Region, &l.Province, &l.City, &l.ISP); err != nil {
		return nil, nil
	}
	logger.L().Debug("db_lookup_done", "loc_id", locID, "country", l.Country, "region", l.Region, "province", l.Province, "city", l.City)
	return &l, nil
}

func (s *Store) LookupKV(ctx context.Context, ip string) (*Location, error) {
	val, err := ipToInt(ip)
	if err != nil {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx, "SELECT country, region, province, city, isp FROM _ip_overrides_kv WHERE ip_int=$1 LIMIT 1", int64(val))
	var l Location
	if err := row.Scan(&l.Country, &l.Region, &l.Province, &l.City, &l.ISP); err != nil {
		return nil, nil
	}
	return &l, nil
}

// IncrStats: 成功查询后递增总计与当日计数；访客存在时递增访客计数
func (s *Store) IncrStats(ctx context.Context, visitor string) error {
	_, _ = s.db.ExecContext(ctx, "UPDATE _ip_stats_total SET total_queries=total_queries+1 WHERE id=1")
	_, _ = s.db.ExecContext(ctx, "INSERT INTO _ip_stats_daily(day, queries) VALUES(current_date, 1) ON CONFLICT (day) DO UPDATE SET queries=_ip_stats_daily.queries+1")
	if visitor != "" {
		_, _ = s.db.ExecContext(ctx, "UPDATE _ip_stats_total SET total_visitors=total_visitors+1 WHERE id=1")
		_, _ = s.db.ExecContext(ctx, "INSERT INTO _ip_stats_daily(day, visitors) VALUES(current_date, 1) ON CONFLICT (day) DO UPDATE SET visitors=_ip_stats_daily.visitors+1")
	}
	logger.L().Debug("stats_incr", "visitor", visitor)
	return nil
}

// Totals: 统计返回结构，包含累计与当日查询次数
type Totals struct {
	Total int64
	Today int64
}

// GetTotals: 读取累计与当日查询次数，用于接口返回
func (s *Store) GetTotals(ctx context.Context) (*Totals, error) {
	var t Totals
	row := s.db.QueryRowContext(ctx, "SELECT total_queries FROM _ip_stats_total WHERE id=1")
	_ = row.Scan(&t.Total)
	row2 := s.db.QueryRowContext(ctx, "SELECT queries FROM _ip_stats_daily WHERE day=current_date")
	_ = row2.Scan(&t.Today)
	logger.L().Debug("stats_totals", "total", t.Total, "today", t.Today)
	return &t, nil
}
