// 包 api：集中注册 HTTP API 路由以解耦主入口，便于后续扩展与替换
// NOTE: 路由构建保持最小依赖，仅以 Store/Redis/缓存接口为边界，减少主进程耦合与测试复杂度。
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"ip-api/internal/fusion"
	"ip-api/internal/ingest"
	"ip-api/internal/localdb"
	"ip-api/internal/localdb/chain"
	"ip-api/internal/localdb/exact"
	ip2region "ip-api/internal/localdb/ip2region"
	ipipcache "ip-api/internal/localdb/ipip"
	"ip-api/internal/logger"
	"ip-api/internal/metrics"
	"ip-api/internal/plugins"
	"ip-api/internal/store"
	"ip-api/internal/version"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// WARNING: 代理头可能被伪造，部署时需结合可信代理列表或网关过滤，避免滥用导致去重与统计偏差。
// 文档注释：构建并返回 API 路由（插件融合版）
// 背景：引入插件管理器与动态缓存热切换；在缓存与数据库回退不足时并发调用插件，融合后按策略写库并触发重建。
// 参数：
// - st：数据库访问入口；用于 KV 优先与范围回退，以及写入与统计；
// - rc：Redis 客户端（可选）；用于热点缓存与布隆去重；
// - dc：动态缓存（支持 Lookup/Set）；用于链式缓存原子切换；
// - pm：插件管理器；提供健康插件集合与融合。
func BuildRoutes(st *store.Store, rc *redis.Client, dc *localdb.DynamicCache, pm *plugins.Manager) *http.ServeMux {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		commit := version.Commit
		built := version.BuiltAt
		if commit == "" || strings.ToLower(commit) == "unknown" {
			commit = "dev"
		}
		if strings.ToLower(built) == "unknown" {
			built = ""
		}
		m := map[string]any{"commit": commit, "builtAt": built}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.Header().Set("cache-control", "no-store")
		_ = json.NewEncoder(w).Encode(m)
	})
	apiMux.HandleFunc("/ip", func(w http.ResponseWriter, r *http.Request) {
		l := logger.L()
		ctx := r.Context()
		tBegin := time.Now()
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
		cacheSec := 600
		if s := os.Getenv("CACHE_TTL_SECONDS"); s != "" {
			if n, e := strconv.Atoi(s); e == nil && n > 0 {
				cacheSec = n
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
		// KV 覆盖前置：若命中则直接返回，覆盖文件缓存
		if ip != "" && !isIPv6 {
			if kv, _ := st.LookupKV(ctx, ip); kv != nil {
				res.Country = kv.Country
				res.Region = kv.Region
				res.Province = kv.Province
				res.City = kv.City
				res.ISP = kv.ISP
				if rc != nil {
					b, _ := json.Marshal(res)
					_ = rc.Set(ctx, "ip:"+ip, string(b), time.Duration(cacheSec)*time.Second).Err()
				}
				w.Header().Set("content-type", "application/json; charset=utf-8")
				w.Header().Set("cache-control", "no-store")
				_ = json.NewEncoder(w).Encode(res)
				if added {
					_ = st.IncrStats(ctx, ip)
					_ = st.RecordRecent(ctx, ip)
				}
				metrics.RequestsTotal.Inc()
				metrics.RequestDurationMs.Observe(float64(time.Since(tBegin).Milliseconds()))
				if res.Country == "" && res.Region == "" && res.Province == "" && res.City == "" && res.ISP == "" {
					metrics.EmptyResultsTotal.Inc()
				}
				return
			}
		}
		// 背景：热点查询结果写入 Redis，降低重复请求对下游的压力
		// 约束：过期时间固定 24h；命中后直接返回并累加统计
		if ip != "" && rc != nil {
			s, _ := rc.Get(ctx, "ip:"+ip).Result()
			if s != "" {
				l.Debug("cache_hit", "key", "ip:"+ip)
				metrics.RedisHitsTotal.Inc()
				_ = json.Unmarshal([]byte(s), &res)
				// 命中但字段不完整时可触发融合
				if os.Getenv("ENABLE_FUSION_ON_PARTIAL_CACHE") == "true" && pm != nil && !isIPv6 {
					if res.Province == "" || res.City == "" {
						ctx2, cancel := context.WithTimeout(ctx, 4*time.Second)
						loc, score, conf, top := pm.Aggregate(ctx2, ip)
						cancel()
						if loc.Country != "" || loc.Region != "" || loc.Province != "" || loc.City != "" || loc.ISP != "" {
							minScore := 20
							if s := os.Getenv("FUSION_MIN_SCORE_ON_CACHE"); s != "" {
								if n, e := strconv.Atoi(s); e == nil && n > 0 {
									minScore = n
								}
							}
							oldComplete := res.Province != "" && res.City != ""
							newComplete := loc.Province != "" && loc.City != ""
							if (score >= float64(minScore)) || (!oldComplete && newComplete) || (oldComplete && newComplete) {
								res.Country = loc.Country
								res.Region = loc.Region
								res.Province = loc.Province
								res.City = loc.City
								res.ISP = loc.ISP
								assoc := "global"
								if top != nil && top.Assoc != "" {
									assoc = top.Assoc
								}
								logger.L().Debug("plugin_fusion_on_cache", "score", score, "conf", conf, "assoc", assoc)
								_ = st.UpsertOverrideKV(ctx, assoc, ip, ingest.Location{Country: loc.Country, Region: loc.Region, Province: loc.Province, City: loc.City, ISP: loc.ISP}, score, conf)
								_, wExact := fusion.DecideWrite(fusion.Location{Country: loc.Country, Region: loc.Region, Province: loc.Province, City: loc.City, ISP: loc.ISP}, score)
								if wExact {
									if p := net.ParseIP(ip); p != nil && p.To4() != nil {
										v := p.To4()
										ipInt := uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
										_ = ingest.WriteExact(ctx, st.DB(), ipInt, ingest.Location{Country: loc.Country, Region: loc.Region, Province: loc.Province, City: loc.City, ISP: loc.ISP}, assoc)
										logger.L().Debug("plugin_write_exact", "ip", ip, "assoc", assoc)
									}
								}
								if rc != nil {
									b, _ := json.Marshal(res)
									_ = rc.Set(ctx, "ip:"+ip, string(b), time.Duration(cacheSec)*time.Second).Err()
									logger.L().Debug("cache_set", "key", "ip:"+ip, "len", len(b), "ttl_s", cacheSec)
								}
								go func() {
									fileDir := "data/localdb"
									if err := exactRebuildAndSwitch(st.DB(), dc, fileDir); err != nil {
										logger.L().Error("exact_rebuild_switch_error", "err", err)
									} else {
										logger.L().Info("exact_rebuild_switch_ok")
									}
								}()
							} else {
								logger.L().Debug("plugin_fusion_skip_cache", "score", score, "min", minScore, "old_complete", oldComplete, "new_complete", newComplete)
							}
						}
					}
				}
				if os.Getenv("ENABLE_FUSION_ON_PARTIAL_CACHE") != "true" && (res.Province == "" || res.City == "") {
					logger.L().Debug("cache_hit_partial_skip_env_off")
				}
				if os.Getenv("ENABLE_FUSION_ON_PARTIAL_CACHE") == "true" && !(res.Province == "" || res.City == "") {
					logger.L().Debug("cache_hit_partial_skip_complete")
				}
				w.Header().Set("content-type", "application/json; charset=utf-8")
				w.Header().Set("cache-control", "no-store")
				_ = json.NewEncoder(w).Encode(res)
				if added {
					_ = st.IncrStats(ctx, ip)
					_ = st.RecordRecent(ctx, ip)
				}
				metrics.RequestsTotal.Inc()
				metrics.RequestDurationMs.Observe(float64(time.Since(tBegin).Milliseconds()))
				if res.Country == "" && res.Region == "" && res.Province == "" && res.City == "" && res.ISP == "" {
					metrics.EmptyResultsTotal.Inc()
				}
				return
			}
			l.Debug("cache_miss", "key", "ip:"+ip)
			metrics.RedisMissesTotal.Inc()
		}
		// 背景：优先使用本地压缩内存缓存快速读取（IPv4）；失败回退数据库
		tFileBegin := time.Now()
		if dc != nil && ip != "" && !isIPv6 {
			if l, ok := dc.Lookup(ip); ok {
				res.Country = l.Country
				res.Region = l.Region
				res.Province = l.Province
				res.City = l.City
				res.ISP = l.ISP
				logger.L().Debug("localdb_hit")
				w.Header().Set("x-step-ms-file", strconv.FormatInt(time.Since(tFileBegin).Milliseconds(), 10))
				if os.Getenv("ENABLE_FUSION_ON_PARTIAL_CACHE") == "true" && pm != nil {
					if res.Province == "" || res.City == "" {
						ctx2, cancel := context.WithTimeout(ctx, 4*time.Second)
						loc, score, conf, top := pm.Aggregate(ctx2, ip)
						cancel()
						if loc.Country != "" || loc.Region != "" || loc.Province != "" || loc.City != "" || loc.ISP != "" {
							minScore := 20
							if s := os.Getenv("FUSION_MIN_SCORE_ON_CACHE"); s != "" {
								if n, e := strconv.Atoi(s); e == nil && n > 0 {
									minScore = n
								}
							}
							oldComplete := res.Province != "" && res.City != ""
							newComplete := loc.Province != "" && loc.City != ""
							if (score >= float64(minScore)) || (!oldComplete && newComplete) || (oldComplete && newComplete) {
								res.Country = loc.Country
								res.Region = loc.Region
								res.Province = loc.Province
								res.City = loc.City
								res.ISP = loc.ISP
								assoc := "global"
								if top != nil && top.Assoc != "" {
									assoc = top.Assoc
								}
								logger.L().Debug("plugin_fusion_on_localdb", "score", score, "conf", conf, "assoc", assoc)
								_ = st.UpsertOverrideKV(ctx, assoc, ip, ingest.Location{Country: loc.Country, Region: loc.Region, Province: loc.Province, City: loc.City, ISP: loc.ISP}, score, conf)
								_, wExact := fusion.DecideWrite(fusion.Location{Country: loc.Country, Region: loc.Region, Province: loc.Province, City: loc.City, ISP: loc.ISP}, score)
								if wExact {
									if p := net.ParseIP(ip); p != nil && p.To4() != nil {
										v := p.To4()
										ipInt := uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
										_ = ingest.WriteExact(ctx, st.DB(), ipInt, ingest.Location{Country: loc.Country, Region: loc.Region, Province: loc.Province, City: loc.City, ISP: loc.ISP}, assoc)
										logger.L().Debug("plugin_write_exact", "ip", ip, "assoc", assoc)
									}
								}
								if os.Getenv("ENABLE_FUSION_ON_PARTIAL_CACHE") != "true" && (res.Province == "" || res.City == "") {
									logger.L().Debug("localdb_partial_skip_env_off")
								}
								if os.Getenv("ENABLE_FUSION_ON_PARTIAL_CACHE") == "true" && !(res.Province == "" || res.City == "") {
									logger.L().Debug("localdb_partial_skip_complete")
								}
								if rc != nil {
									b, _ := json.Marshal(res)
									_ = rc.Set(ctx, "ip:"+ip, string(b), time.Duration(cacheSec)*time.Second).Err()
									logger.L().Debug("cache_set", "key", "ip:"+ip, "len", len(b), "ttl_s", cacheSec)
								}
								go func() {
									fileDir := "data/localdb"
									if err := exactRebuildAndSwitch(st.DB(), dc, fileDir); err != nil {
										logger.L().Error("exact_rebuild_switch_error", "err", err)
									} else {
										logger.L().Info("exact_rebuild_switch_ok")
									}
								}()
							} else {
								logger.L().Debug("plugin_fusion_skip_localdb", "score", score, "min", minScore, "old_complete", oldComplete, "new_complete", newComplete)
							}
						}
					}
				}
				if rc != nil {
					if res.Country != "" || res.Region != "" || res.Province != "" || res.City != "" || res.ISP != "" {
						b, _ := json.Marshal(res)
						_ = rc.Set(ctx, "ip:"+ip, string(b), time.Duration(cacheSec)*time.Second).Err()
						logger.L().Debug("cache_set", "key", "ip:"+ip, "len", len(b), "ttl_s", cacheSec)
					} else {
						logger.L().Debug("cache_skip_empty", "key", "ip:"+ip)
					}
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
					_ = st.RecordRecent(ctx, ip)
				}
				w.Header().Set("content-type", "application/json; charset=utf-8")
				w.Header().Set("cache-control", "no-store")
				_ = json.NewEncoder(w).Encode(res)
				metrics.RequestsTotal.Inc()
				metrics.RequestDurationMs.Observe(float64(time.Since(tBegin).Milliseconds()))
				if res.Country == "" && res.Region == "" && res.Province == "" && res.City == "" && res.ISP == "" {
					metrics.EmptyResultsTotal.Inc()
				}
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
					if res.Country != "" || res.Region != "" || res.Province != "" || res.City != "" || res.ISP != "" {
						b, _ := json.Marshal(res)
						_ = rc.Set(ctx, "ip:"+ip, string(b), time.Duration(cacheSec)*time.Second).Err()
						logger.L().Debug("cache_set", "key", "ip:"+ip, "len", len(b), "ttl_s", cacheSec)
					} else {
						logger.L().Debug("cache_skip_empty", "key", "ip:"+ip)
					}
				}
				if added {
					_ = st.IncrStats(ctx, ip)
					_ = st.RecordRecent(ctx, ip)
				}
			}
			if loc == nil {
				logger.L().Debug("db_range_miss")
			}
		}
		// 插件融合：DB 未命中或命中但字段不完整且开关启用时触发
		triggerFusion := ip != "" && !isIPv6 && pm != nil
		if res.Country != "" || res.Region != "" || res.Province != "" || res.City != "" || res.ISP != "" {
			needOnPartial := os.Getenv("ENABLE_FUSION_ON_PARTIAL_DB") == "true"
			incomplete := (res.Province == "" || res.City == "")
			if !needOnPartial || !incomplete {
				triggerFusion = false
			} else {
				logger.L().Debug("fusion_on_partial_db", "ip", ip)
			}
		}
		if triggerFusion {
			ctx2, cancel := context.WithTimeout(ctx, 4*time.Second)
			loc, score, conf, top := pm.Aggregate(ctx2, ip)
			cancel()
			if loc.Country != "" || loc.Region != "" || loc.Province != "" || loc.City != "" || loc.ISP != "" {
				res.Country = loc.Country
				res.Region = loc.Region
				res.Province = loc.Province
				res.City = loc.City
				res.ISP = loc.ISP
				assoc := "global"
				if top != nil && top.Assoc != "" {
					assoc = top.Assoc
				}
				logger.L().Debug("plugin_fusion_hit", "score", score, "conf", conf, "assoc", assoc)
				_ = st.UpsertOverrideKV(ctx, assoc, ip, ingest.Location{Country: loc.Country, Region: loc.Region, Province: loc.Province, City: loc.City, ISP: loc.ISP}, score, conf)
				_, wExact := fusion.DecideWrite(fusion.Location{Country: loc.Country, Region: loc.Region, Province: loc.Province, City: loc.City, ISP: loc.ISP}, score)
				if wExact {
					if p := net.ParseIP(ip); p != nil && p.To4() != nil {
						v := p.To4()
						ipInt := uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
						_ = ingest.WriteExact(ctx, st.DB(), ipInt, ingest.Location{Country: loc.Country, Region: loc.Region, Province: loc.Province, City: loc.City, ISP: loc.ISP}, assoc)
						logger.L().Debug("plugin_write_exact", "ip", ip, "assoc", assoc)
					}
				}
				if rc != nil {
					b, _ := json.Marshal(res)
					_ = rc.Set(ctx, "ip:"+ip, string(b), time.Duration(cacheSec)*time.Second).Err()
					logger.L().Debug("cache_set", "key", "ip:"+ip, "len", len(b), "ttl_s", cacheSec)
				}
				go func() {
					// 重建 ExactDB 并原子热切换链式缓存（ExactDB → IPIP → IP2Region）
					fileDir := "data/localdb"
					if err := exactRebuildAndSwitch(st.DB(), dc, fileDir); err != nil {
						logger.L().Error("exact_rebuild_switch_error", "err", err)
					} else {
						logger.L().Info("exact_rebuild_switch_ok")
					}
				}()
			}
		}
		// 文档注释：统一触发融合（在存在 EdgeOne 城市/区域时）
		// 背景：避免因缓存/本地库命中而绕过融合，导致跨源拼接；当上下文中存在 EdgeOne 的强信号（城市/区域）时强制触发融合以稳定整组输出。
		if pm != nil && !isIPv6 {
			v := ctx.Value("edgeone_geo")
			if v != nil {
				if g, ok := v.(plugins.EdgeOneGeoInfo); ok {
					if g.CityName != "" || g.RegionName != "" {
						ctx2, cancel := context.WithTimeout(ctx, 4*time.Second)
						loc, score, conf, top := pm.Aggregate(ctx2, ip)
						cancel()
						if loc.Country != "" || loc.Region != "" || loc.Province != "" || loc.City != "" || loc.ISP != "" {
							res.Country = loc.Country
							res.Region = loc.Region
							res.Province = loc.Province
							res.City = loc.City
							res.ISP = loc.ISP
							assoc := "global"
							if top != nil && top.Assoc != "" {
								assoc = top.Assoc
							}
							logger.L().Debug("plugin_fusion_force_edgeone", "score", score, "conf", conf, "assoc", assoc)
						} else {
							logger.L().Debug("plugin_fusion_force_edgeone_skip_empty")
						}
					}
				}
			}
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.Header().Set("cache-control", "no-store")
		if ip != "" {
			w.Header().Set("x-client-ip", ip)
			w.Header().Set("Access-Control-Expose-Headers", "x-client-ip")
		}
		// 文档注释：终端兜底守护（一致性）
		// 背景：在响应构造阶段进行一致性校验，当区域/城市明显属于中国而国家非中国时，执行兜底修正，避免跨源拼接造成的矛盾对外输出。
		if fusion.CoherenceCoeff(fusion.Location{Country: res.Country, Region: res.Region, Province: res.Province, City: res.City, ISP: res.ISP}) < 1.0 {
			logger.L().Info("api_country_fallback_applied", "prev_country", res.Country, "region", res.Region, "city", res.City)
			res.Country = "中国"
		}
		_ = json.NewEncoder(w).Encode(res)
		if ip != "" {
			_ = st.RecordRecent(ctx, ip)
		}
		metrics.RequestsTotal.Inc()
		metrics.RequestDurationMs.Observe(float64(time.Since(tBegin).Milliseconds()))
		if res.Country == "" && res.Region == "" && res.Province == "" && res.City == "" && res.ISP == "" {
			metrics.EmptyResultsTotal.Inc()
		}
	})

	// 反地理查询接口改为内部调用，不再对外暴露 HTTP 路由

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

// 文档注释：重建 ExactDB 并原子热切换动态缓存
// 背景：写库成功后异步重建精确文件并切换链式缓存，避免并发阻塞与服务中断。
// 约束：IPIP 路径与 IP2Region v4 路径通过环境变量提供；失败时保持现状不切换。
func exactRebuildAndSwitch(db *sql.DB, dc *localdb.DynamicCache, dir string) error {
	if dc == nil {
		return nil
	}
	if err := exact.BuildExactDBFromDB(dir, db); err != nil {
		return err
	}
	edb, err := exact.NewExactDB(dir, db)
	if err != nil {
		return err
	}
	var iptree interface {
		Lookup(string) (localdb.Location, bool)
	}
	var ip2r interface {
		Lookup(string) (localdb.Location, bool)
	}
	lang := os.Getenv("IPIP_LANG")
	if lang == "" {
		lang = "zh-CN"
	}
	if p := os.Getenv("IPIP_PATH"); p != "" {
		if c, err := ipipcache.NewIPIPCache(p, lang); err == nil {
			iptree = c
		}
	}
	if p := os.Getenv("IP2REGION_V4_PATH"); p != "" {
		if c, err := ip2region.NewIP2RegionCache(p, ""); err == nil {
			ip2r = c
		}
	}
	mc := chain.NewChainCache(edb, iptree, ip2r)
	dc.Set(mc)
	return nil
}
