package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"ip-api/internal/fusion"
	"ip-api/internal/ingest"
	"ip-api/internal/localdb"
	"ip-api/internal/localdb/ip2region"
	"ip-api/internal/localdb/ipip"
	"ip-api/internal/logger"
	"ip-api/internal/store"
	"ip-api/internal/utils"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// 文档注释：将 IPv4 文本转换为整数
// 背景：写库时统一使用整数主键，避免文本处理带来的索引不一致；非法输入返回错误。
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

// 文档注释：覆盖 KV 表写入（amap 来源）
// 背景：优先通过 KV 覆盖让 API 首步命中；assoc_key 用于来源标识与分域管理。
func upsertKV(ctx context.Context, db *sql.DB, assocKey string, ip string, country, region, province, city, isp string, score float64, confidence float64) error {
	v, err := ipToInt(ip)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO _ip_overrides_kv(assoc_key, ip_int, country, region, province, city, isp, score, confidence)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
        ON CONFLICT (assoc_key, ip_int) DO UPDATE SET country=EXCLUDED.country, region=EXCLUDED.region, province=EXCLUDED.province, city=EXCLUDED.city, isp=EXCLUDED.isp, score=EXCLUDED.score, confidence=EXCLUDED.confidence, updated_at=now()
        WHERE COALESCE(_ip_overrides_kv.score, 0) + 20 <= EXCLUDED.score`,
		assocKey, int64(v), country, region, province, city, isp, score, confidence,
	)
	return err
}

// 文档注释：简单令牌桶限流（每分钟）
// 背景：受外部配额限制，控制每分钟最大请求数；超出时阻塞等待下一分钟刷新。
type minuteLimiter struct {
	capacity int
	used     int
	lastMin  int64
	mu       sync.Mutex
}

func (ml *minuteLimiter) allow() bool {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	nowMin := time.Now().Unix() / 60
	if ml.lastMin != nowMin {
		ml.lastMin = nowMin
		ml.used = 0
	}
	if ml.used < ml.capacity {
		ml.used++
		return true
	}
	return false
}

func main() {
	_ = godotenv.Load(".env")
	l := logger.Setup()
	l.Info("amap_ingest_start")
	key := os.Getenv("AMAP_SERVER_KEY")
	if key == "" {
		l.Error("amap_key_missing")
		os.Exit(1)
	}
	db, err := utils.OpenPostgresFromEnv()
	if err != nil {
		l.Error("db_open_error", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		l.Error("db_ping_error", "err", err)
	}

	// 输入源：数据库最近查询候选（默认）；可选文件/标准输入
	inPath := os.Getenv("AMAP_INPUT_FILE")
	useDB := strings.ToLower(os.Getenv("AMAP_SOURCE"))
	if useDB == "" {
		useDB = "db"
	}

	// 并发与限流
	workers := 4
	if v := os.Getenv("AMAP_WORKERS"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			workers = n
		}
	}
	ratePerMin := 120
	if v := os.Getenv("AMAP_RATE_LIMIT_PER_MIN"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			ratePerMin = n
		}
	}
	writeExact := strings.ToLower(os.Getenv("AMAP_WRITE_EXACT")) == "true"
	assocKey := os.Getenv("AMAP_ASSOC_KEY")
	if assocKey == "" {
		assocKey = "amap"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	// 聚合数据源初始化
	lang := os.Getenv("IPIP_LANG")
	if lang == "" {
		lang = "zh-CN"
	}
	var ipipCache interface {
		Lookup(string) (localdb.Location, bool)
	}
	if p := os.Getenv("IPIP_PATH"); p != "" {
		if c, err := ipip.NewIPIPCache(p, lang); err == nil {
			ipipCache = c
		} else {
			logger.L().Error("ipip_cache_error", "err", err)
		}
	}
	st := store.AttachDB(db)
	kvSrc := &fusion.KVSource{Store: st}
	amapSrc := &fusion.AMapSource{Key: key, Client: client}
	ipipSrc := &fusion.IPIPSource{Cache: ipipCache}
	var ip2rSrc fusion.DataSource
	if p := os.Getenv("IP2REGION_V4_PATH"); p != "" {
		if c, err := ip2region.NewIP2RegionCache(p, ""); err == nil {
			ip2rSrc = &fusion.IP2RSource{Cache: c}
		} else {
			logger.L().Error("ip2region_init_error", "err", err)
		}
	}
	sources := []fusion.DataSource{kvSrc, ipipSrc}
	if ip2rSrc != nil {
		sources = append(sources, ip2rSrc)
	}
	sources = append(sources, amapSrc)
	limiter := &minuteLimiter{capacity: ratePerMin}

	// 任务派发
	type job struct{ ip string }
	jobs := make(chan job, workers*4)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range jobs {
				// 限流阻塞直到允许
				for !limiter.allow() {
					time.Sleep(250 * time.Millisecond)
				}
				// 聚合查询
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				loc, score, conf := fusion.Aggregate(ctx, sources, j.ip)
				cancel()
				if loc.Province == "" && loc.City == "" {
					logger.L().Warn("fusion_skip_empty", "ip", j.ip)
					continue
				}
				// 写库：优先 KV 覆盖；高分可写精确表
				if err := upsertKV(context.Background(), db, assocKey, j.ip, loc.Country, loc.Region, loc.Province, loc.City, loc.ISP, score, conf); err != nil {
					logger.L().Error("kv_upsert_error", "ip", j.ip, "err", err)
					continue
				}
				if writeExact {
					_, wExact := fusion.DecideWrite(fusion.Location{Country: loc.Country, Region: loc.Region, Province: loc.Province, City: loc.City, ISP: loc.ISP}, score)
					if wExact {
						if v, e := ipToInt(j.ip); e == nil {
							_ = ingest.WriteExact(context.Background(), db, v, ingest.Location{Country: loc.Country, Region: loc.Region, Province: loc.Province, City: loc.City, ISP: loc.ISP}, "amap")
						}
					}
				}
				logger.L().Debug("fusion_ingest_ok", "ip", j.ip, "province", loc.Province, "city", loc.City, "score", score)
			}
		}(i)
	}

	total := 0
	if useDB == "db" && inPath == "" {
		hours := 24
		if v := os.Getenv("AMAP_DB_HOURS"); v != "" {
			if n, e := strconv.Atoi(v); e == nil && n > 0 {
				hours = n
			}
		}
		limit := 1000
		if v := os.Getenv("AMAP_DB_LIMIT"); v != "" {
			if n, e := strconv.Atoi(v); e == nil && n > 0 {
				limit = n
			}
		}
		// 直接 SQL 查询候选，筛掉已覆盖/已精确
		rows, err := db.Query(`
            SELECT r.ip_int
            FROM _ip_recent_ips r
            LEFT JOIN _ip_overrides_kv k ON k.ip_int = r.ip_int
            LEFT JOIN _ip_exact e ON e.ip_int = r.ip_int
            WHERE r.last_seen >= now() - make_interval(hours => $1)
              AND k.ip_int IS NULL
              AND e.ip_int IS NULL
            ORDER BY r.last_seen DESC
            LIMIT $2`, hours, limit)
		if err != nil {
			l.Error("db_source_query_error", "err", err)
			os.Exit(1)
		}
		defer rows.Close()
		for rows.Next() {
			var v int64
			if err := rows.Scan(&v); err != nil {
				l.Error("db_source_scan_error", "err", err)
				os.Exit(1)
			}
			a := (v >> 24) & 0xff
			b := (v >> 16) & 0xff
			c := (v >> 8) & 0xff
			d := v & 0xff
			ip := fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
			jobs <- job{ip: ip}
			total++
		}
		close(jobs)
	} else {
		var rd *bufio.Scanner
		if inPath != "" {
			f, e := os.Open(inPath)
			if e != nil {
				l.Error("input_open_error", "err", e)
				os.Exit(1)
			}
			defer f.Close()
			rd = bufio.NewScanner(f)
		} else {
			rd = bufio.NewScanner(os.Stdin)
		}
		rd.Buffer(make([]byte, 1024), 1024*1024)
		for rd.Scan() {
			line := strings.TrimSpace(rd.Text())
			if line == "" {
				continue
			}
			jobs <- job{ip: line}
			total++
		}
		close(jobs)
		if err := rd.Err(); err != nil {
			l.Error("input_read_error", "err", err)
		}
	}
	wg.Wait()
	l.Info("amap_ingest_done", "total", total)
}
