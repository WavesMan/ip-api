// 包 api：集中注册 HTTP API 路由以解耦主入口，便于后续扩展与替换
// NOTE: 路由构建保持最小依赖，仅以 Store/Redis/缓存接口为边界，减少主进程耦合与测试复杂度。
package api

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"ip-api/internal/ingest"
	"ip-api/internal/localdb"
	"ip-api/internal/logger"
	"ip-api/internal/store"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// 查询结果结构：仅包含对外返回必要字段
type queryResult struct {
	IP       string `json:"ip"`
	Country  string `json:"country"`
	Region   string `json:"region"`
	Province string `json:"province"`
	City     string `json:"city"`
	ISP      string `json:"isp"`
}

// 文档注释：查询返回结构
// 背景：统一对外序列化模型，避免泄露内部字段或不同数据源差异；便于缓存与统计一致化。
// 约束：字段稳定，新增字段需评估兼容性与前端依赖；不在此处承载内部定位精度或来源信息。

// 解析访问者 IP：优先参数，其次常见反向代理头；保证在多层代理场景下稳定获取源 IP
// 文档注释：获取客户端 IP（用于业务查询参数）
// 背景：多层代理环境下，优先显式参数，其次常见反向代理头，最后回退远端地址；确保在复杂链路中得到稳定的来源 IP。
// 约束：不解析 IPv6 压缩形式的特殊头部变体；当头部存在伪造风险时，需在上游做可信代理白名单处理。
func getClientIP(r *http.Request) string {
	q := r.URL.Query().Get("ip")
	if q != "" {
		return q
	}
	h := r.Header
	if x := h.Get("x-forwarded-for"); x != "" {
		return strings.Split(x, ",")[0]
	}
	if x := h.Get("cf-connecting-ip"); x != "" {
		return x
	}
	if x := h.Get("x-real-ip"); x != "" {
		return x
	}
	if x := h.Get("x-client-ip"); x != "" {
		return x
	}
	if x := h.Get("x-edge-client-ip"); x != "" {
		return x
	}
	if x := h.Get("x-edgeone-ip"); x != "" {
		return x
	}
	if x := h.Get("forwarded"); x != "" {
		i := strings.Index(strings.ToLower(x), "for=")
		if i >= 0 {
			y := x[i+4:]
			y = strings.Trim(y, "\" ")
			if p := strings.IndexByte(y, ';'); p >= 0 {
				y = y[:p]
			}
			if p := strings.IndexByte(y, ','); p >= 0 {
				y = y[:p]
			}
			return y
		}
	}
	host := r.RemoteAddr
	if host != "" {
		if i := strings.LastIndex(host, ":"); i > 0 {
			return host[:i]
		}
		return host
	}
	return ""
}

// 文档注释：获取访问者 IP（用于去重与限流）
// 背景：与 getClientIP 分离，避免因查询目标与访问来源混淆导致去重不准；用于布隆去重键的组成。
// 约束：同样依赖常见代理头顺序，若部署于未经信任的代理链路，需要配合网关过滤与鉴权策略。
func getVisitorIP(r *http.Request) string {
	h := r.Header
	if x := h.Get("x-forwarded-for"); x != "" {
		return strings.Split(x, ",")[0]
	}
	if x := h.Get("cf-connecting-ip"); x != "" {
		return x
	}
	if x := h.Get("x-real-ip"); x != "" {
		return x
	}
	if x := h.Get("x-client-ip"); x != "" {
		return x
	}
	if x := h.Get("x-edge-client-ip"); x != "" {
		return x
	}
	if x := h.Get("x-edgeone-ip"); x != "" {
		return x
	}
	if x := h.Get("forwarded"); x != "" {
		i := strings.Index(strings.ToLower(x), "for=")
		if i >= 0 {
			y := x[i+4:]
			y = strings.Trim(y, "\" ")
			if p := strings.IndexByte(y, ';'); p >= 0 {
				y = y[:p]
			}
			if p := strings.IndexByte(y, ','); p >= 0 {
				y = y[:p]
			}
			return y
		}
	}
	host := r.RemoteAddr
	if host != "" {
		if i := strings.LastIndex(host, ":"); i > 0 {
			return host[:i]
		}
		return host
	}
	return ""
}

// 文档注释：计算布隆过滤器位置
// 参数：data 为参与哈希的字节序列，m 为位图大小（建议 2 的幂以便分布更均匀），k 为哈希次数（控制误判率与写入开销）。
// 返回：长度为 k 的位置数组（int64），用于后续 GetBit/SetBit。
// 约束：使用 FNV64a 结合索引扰动；误判率与 m/k 配置相关，需在生产按 QPS/TTL 实测调参。
func bloomPositions(data []byte, m uint32, k int) []int64 {
	pos := make([]int64, k)
	for i := 0; i < k; i++ {
		h := fnv.New64a()
		h.Write([]byte{byte(i)})
		h.Write(data)
		v := h.Sum64()
		p := uint32(v % uint64(m))
		pos[i] = int64(p)
	}
	return pos
}

// 文档注释：检查并写入布隆过滤器位图
// 背景：用于短周期去重，降低重复请求对后端与缓存的压力；命中视为“已见过”，不再重复处理。
// 返回：true 表示首次见到（已写入位图，可继续处理）；false 表示已存在（建议直接快速返回或限频）。
// 异常：Redis 交互错误时返回 error；当 rc 为 nil 时视为“允许处理”，避免阻断主流程。
func bloomCheckAndSet(ctx context.Context, rc *redis.Client, key string, positions []int64, ttl time.Duration) (bool, error) {
	if rc == nil {
		return true, nil
	}
	seen := true
	for _, p := range positions {
		b, err := rc.GetBit(ctx, key, p).Result()
		if err != nil {
			return true, err
		}
		if b == 0 {
			seen = false
		}
	}
	if !seen {
		for _, p := range positions {
			_, _ = rc.SetBit(ctx, key, p, 1).Result()
		}
		_ = rc.Expire(ctx, key, ttl).Err()
		return true, nil
	}
	return false, nil
}

// 构建并返回 API 路由：独立 ServeMux 便于在主入口挂载到 /api 前缀
// 文档注释：构建并返回 API 路由
// 背景：以独立 ServeMux 暴露 API，主入口仅负责挂载到固定前缀，减少耦合并便于替换；路由内整合缓存/数据库/去重。
// 参数：
// - st：持久化存储访问入口（统计与范围查询）；
// - rc：Redis 客户端（可选，用于热点缓存与布隆去重）；
// - cache：本地压缩/文件缓存接口（Lookup 只处理 IPv4）。
// 返回：完成注册的 ServeMux，由主进程决定最终挂载路径。
// 约束：
// - 去重 TTL 通过环境变量配置（DEDUP_TTL_SECONDS），需结合负载与业务体验调优；
// - 缓存键命名固定前缀（ip:），便于统一清理；
// - IPv6 目前仅透传 IP，不参与本地范围匹配。
// WARNING: 代理头可能被伪造，部署时需结合可信代理列表或网关过滤，避免滥用导致去重与统计偏差。
func BuildRoutes(st *store.Store, rc *redis.Client, cache interface {
	Lookup(string) (localdb.Location, bool)
}) *http.ServeMux {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/ip", func(w http.ResponseWriter, r *http.Request) {
		l := logger.L()
		ctx := r.Context()
		ip := r.URL.Query().Get("ip")
		if ip == "" {
			ip = getClientIP(r)
		}
		visitor := getVisitorIP(r)
		ua := r.Header.Get("User-Agent")
		ttlSec := 600
		if s := os.Getenv("DEDUP_TTL_SECONDS"); s != "" {
			if n, e := strconv.Atoi(s); e == nil && n > 0 {
				ttlSec = n
			}
		}
		bucket := time.Now().Unix() / int64(ttlSec)
		bfKey := "bf:dedupe:" + strconv.FormatInt(bucket, 10)
		added := true
		if ip != "" && visitor != "" {
			id := visitor + "|" + ip + "|" + ua
			positions := bloomPositions([]byte(id), 262144, 4)
			a, _ := bloomCheckAndSet(ctx, rc, bfKey, positions, time.Duration(ttlSec)*time.Second)
			added = a
		}
		isIPv6 := false
		if p := net.ParseIP(ip); p != nil {
			isIPv6 = p.To4() == nil
		}
		l.Debug("api_ip_query", "ip", ip, "ipv6", isIPv6)
		var res queryResult
		res.IP = ip
		// 背景：热点查询结果写入 Redis，降低重复请求对下游的压力
		// 约束：过期时间固定 24h；命中后直接返回并累加统计
		if ip != "" && rc != nil {
			s, _ := rc.Get(ctx, "ip:"+ip).Result()
			if s != "" {
				l.Debug("cache_hit", "key", "ip:"+ip)
				_ = json.Unmarshal([]byte(s), &res)
				w.Header().Set("content-type", "application/json; charset=utf-8")
				w.Header().Set("cache-control", "no-store")
				_ = json.NewEncoder(w).Encode(res)
				if added {
					_ = st.IncrStats(ctx, ip)
				}
				return
			}
			l.Debug("cache_miss", "key", "ip:"+ip)
		}
		// 背景：优先使用本地压缩内存缓存快速读取（IPv4）；失败回退数据库
		tFileBegin := time.Now()
		if cache != nil && ip != "" && !isIPv6 {
			if l, ok := cache.Lookup(ip); ok {
				res.Country = l.Country
				res.Region = l.Region
				res.Province = l.Province
				res.City = l.City
				res.ISP = l.ISP
				logger.L().Debug("localdb_hit")
				w.Header().Set("x-step-ms-file", strconv.FormatInt(time.Since(tFileBegin).Milliseconds(), 10))
				if rc != nil {
					b, _ := json.Marshal(res)
					rc.Set(ctx, "ip:"+ip, string(b), time.Hour*24)
				}
				go func() {
					if p := net.ParseIP(ip); p != nil && p.To4() != nil {
						v := p.To4()
						ipInt := uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
						logger.L().Debug("lazy_exact_persist", "ip", ip)
						_ = ingest.WriteExact(ctx, st.DB(), ipInt, ingest.Location{Country: l.Country, Region: l.Region, Province: l.Province, City: l.City, ISP: l.ISP}, "filecache")
					}
				}()
				if added {
					_ = st.IncrStats(ctx, ip)
				}
				w.Header().Set("content-type", "application/json; charset=utf-8")
				w.Header().Set("cache-control", "no-store")
				_ = json.NewEncoder(w).Encode(res)
				return
			}
			logger.L().Debug("localdb_miss")
		}
		// 背景：数据库范围回退（仅 IPv4）；保障 mmdb 不足或缺席情况下仍可服务
		// 约束：命中后同样写入缓存与统计
		if !isIPv6 && ip != "" {
			tDBBegin := time.Now()
			loc, _ := st.LookupIP(ctx, ip)
			if loc != nil {
				logger.L().Debug("db_range_hit")
				res.Country = loc.Country
				res.Region = loc.Region
				res.Province = loc.Province
				res.City = loc.City
				res.ISP = loc.ISP
				w.Header().Set("x-step-ms-db", strconv.FormatInt(time.Since(tDBBegin).Milliseconds(), 10))
				if rc != nil {
					b, _ := json.Marshal(res)
					rc.Set(ctx, "ip:"+ip, string(b), time.Hour*24)
				}
				if added {
					_ = st.IncrStats(ctx, ip)
				}
			}
			if loc == nil {
				logger.L().Debug("db_range_miss")
			}
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.Header().Set("cache-control", "no-store")
		if ip != "" {
			w.Header().Set("x-client-ip", ip)
			w.Header().Set("Access-Control-Expose-Headers", "x-client-ip")
		}
		_ = json.NewEncoder(w).Encode(res)
	})

	// 背景：提供服务量统计，用于前端展示与简单监控；不做持久化聚合
	apiMux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		t, _ := st.GetTotals(r.Context())
		m := map[string]any{"total": t.Total, "today": t.Today}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.Header().Set("cache-control", "no-store")
		_ = json.NewEncoder(w).Encode(m)
	})

	// // 背景：预留重载接口以重建本地压缩缓存；需管理令牌
	// apiMux.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
	// 	token := r.Header.Get("x-admin-token")
	// 	if token == "" || token != os.Getenv("ADMIN_TOKEN") {
	// 		w.WriteHeader(http.StatusForbidden)
	// 		return
	// 	}
	// 	c, err := localdb.BuildFromDB(st.DB())
	// 	if err != nil {
	// 		w.WriteHeader(http.StatusInternalServerError)
	// 		return
	// 	}
	// 	cache = c
	// 	logger.L().Info("reload_done")
	// 	w.WriteHeader(http.StatusNoContent)
	// })

	return apiMux
}
