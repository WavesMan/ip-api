package plugins

import (
	"context"
	"ip-api/internal/fusion"
	"ip-api/internal/logger"
	"ip-api/internal/metrics"
	"math"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

// 文档注释：插件接口（统一契约）
// 背景：抽象各数据源为同构插件，主服务通过统一接口并发查询与融合；插件侧决定权重与置信度评估。
// 约束：Query 返回字段需为主服务可归一化的 Location；GetWeight 可静态或按 IP 动态；Heartbeat 用于健康检测与熔断。
type Plugin interface {
	Name() string
	Version() string
	AssocKey() string
	Query(ctx context.Context, ip string) (fusion.Location, float64)
	GetWeight(ip string) float64
	Heartbeat(ctx context.Context) error
}

// 文档注释：插件健康状态缓存
// 背景：记录健康与最近心跳时间；管理层据此筛选“健康插件集合”。
type status struct {
	healthy bool
	last    time.Time
}

// 文档注释：插件管理器
// 背景：负责插件注册、心跳、健康筛选；为融合层提供动态可用的插件列表。
// 约束：心跳周期默认 10s；心跳异常视为不健康，自动从集合剔除；线程安全读写。
type Manager struct {
	mu         sync.RWMutex
	ps         map[string]Plugin
	st         map[string]status
	hbInterval time.Duration
}

func NewManager() *Manager {
	return &Manager{ps: make(map[string]Plugin), st: make(map[string]status), hbInterval: 10 * time.Second}
}

// 文档注释：注册插件
// 背景：进程内/外插件均通过此方法注册到管理器；默认设置为健康状态以便参与查询。
func (m *Manager) Register(p Plugin) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ps[p.Name()] = p
	m.st[p.Name()] = status{healthy: true, last: time.Now()}
	logger.L().Info("plugin_registered", "name", p.Name(), "assoc", p.AssocKey(), "version", p.Version())
}

// 文档注释：获取健康插件集合
// 背景：供融合层调用；仅返回当前判定为健康的插件。
func (m *Manager) HealthyPlugins() []Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Plugin
	for k, p := range m.ps {
		s := m.st[k]
		if s.healthy {
			out = append(out, p)
		}
	}
	return out
}

// 文档注释：启动心跳循环
// 背景：周期性调用插件 Heartbeat 更新健康状态；在 ctx 取消时停止。
func (m *Manager) Start(ctx context.Context) {
	t := time.NewTicker(m.hbInterval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.doHeartbeat(ctx)
			}
		}
	}()
}

func (m *Manager) doHeartbeat(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, p := range m.ps {
		err := p.Heartbeat(ctx)
		if err != nil {
			m.st[k] = status{healthy: false, last: time.Now()}
			logger.L().Debug("plugin_heartbeat_fail", "name", p.Name(), "err", err)
			metrics.PluginHeartbeatTotal.WithLabelValues(p.Name(), "fail").Inc()
		} else {
			m.st[k] = status{healthy: true, last: time.Now()}
			logger.L().Debug("plugin_heartbeat_ok", "name", p.Name())
			metrics.PluginHeartbeatTotal.WithLabelValues(p.Name(), "ok").Inc()
		}
	}
}

// 文档注释：内置插件适配器
// 背景：将既有 DataSource 实现包装为插件，复用其查询与权重逻辑；便于平滑迁移。
type BuiltinPlugin struct {
	name    string
	version string
	assoc   string
	src     fusion.DataSource
}

func NewBuiltin(name, version, assoc string, src fusion.DataSource) *BuiltinPlugin {
	return &BuiltinPlugin{name: name, version: version, assoc: assoc, src: src}
}

func (b *BuiltinPlugin) Name() string     { return b.name }
func (b *BuiltinPlugin) Version() string  { return b.version }
func (b *BuiltinPlugin) AssocKey() string { return b.assoc }
func (b *BuiltinPlugin) Query(ctx context.Context, ip string) (fusion.Location, float64) {
	return b.src.Query(ctx, ip)
}
func (b *BuiltinPlugin) GetWeight(ip string) float64         { return b.src.GetWeight() }
func (b *BuiltinPlugin) Heartbeat(ctx context.Context) error { return nil }

// 文档注释：加权结果携带来源关联键
// 背景：用于写库时按来源分域管理（assoc_key）。
type Weighted struct {
	Loc        fusion.Location
	Score      float64
	Confidence float64
	Assoc      string
	Name       string
}

// 文档注释：管理器聚合查询（返回融合与 Top 来源）
// 背景：对健康插件并发查询并计算融合结果；同时选取最高分来源的 assoc_key 用于写库。
// 约束：仅支持包装了 DataSource 的内置插件；外部插件需通过独立路径参与融合另行扩展。
func (m *Manager) Aggregate(ctx context.Context, ip string) (fusion.Location, float64, float64, *Weighted) {
	hs := m.HealthyPlugins()
	logger.L().Debug("plugin_aggregate_begin", "ip", ip, "healthy", len(hs))
	type wr struct {
		Loc   fusion.Location
		Score float64
		Conf  float64
		Assoc string
		Name  string
	}
	var results []wr
	for _, p := range hs {
		t0 := time.Now()
		metrics.PluginRequestsTotal.WithLabelValues(p.Name()).Inc()
		l, c := p.Query(ctx, ip)
		w := p.GetWeight(ip)
		if w > 10 {
			w = 10
		}
		q := qualityCoeff(l)
		co := fusion.CoherenceCoeff(l)
		sc := 100 * (w / 10.0) * q * c * co
		ms := float64(time.Since(t0).Milliseconds())
		metrics.PluginDurationMs.WithLabelValues(p.Name()).Observe(ms)
		if l.Country != "" || l.Region != "" || l.Province != "" || l.City != "" || l.ISP != "" {
			metrics.PluginSuccessTotal.WithLabelValues(p.Name()).Inc()
		} else {
			metrics.PluginFailTotal.WithLabelValues(p.Name()).Inc()
		}
		if co < 1.0 {
			logger.L().Debug("plugin_coherence_penalty_applied", "name", p.Name(), "coeff", co)
		}
		results = append(results, wr{Loc: l, Score: sc, Conf: c, Assoc: p.AssocKey(), Name: p.Name()})
		metrics.PluginScore.WithLabelValues(p.Name()).Observe(sc)
		logger.L().Debug("plugin_weighted", "name", p.Name(), "w", w, "q", q, "c", c, "score", sc)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	top := results
	if len(top) > 3 {
		top = top[:3]
	}
	// 锚定源选择：KV 优先；其次 EdgeOne（有城市/区域且置信度较高）；否则取最高分
	anchorIdx := -1
	for i, r := range top {
		if r.Name == "kv" && (r.Loc.City != "" || r.Loc.Region != "") {
			anchorIdx = i
			break
		}
	}
	if anchorIdx == -1 {
		for i, r := range top {
			if r.Name == "edgeone" && (r.Loc.City != "" || r.Loc.Region != "") && r.Conf >= 0.8 {
				anchorIdx = i
				break
			}
		}
	}
	if anchorIdx == -1 && len(top) > 0 {
		anchorIdx = 0
	}
	var anchor wr
	if anchorIdx >= 0 {
		anchor = top[anchorIdx]
		logger.L().Debug("fusion_anchor_source", "name", anchor.Name, "score", anchor.Score, "conf", anchor.Conf)
	}
	var out fusion.Location
	pick := func(get func(fusion.Location) string, anchorVal string) string {
		if anchorVal != "" {
			return anchorVal
		}
		weights := map[string]float64{}
		for _, r := range top {
			v := get(r.Loc)
			if v != "" {
				weights[v] += r.Score
			}
		}
		var best string
		var bestW float64
		for v, w := range weights {
			if w > bestW {
				bestW = w
				best = v
			}
		}
		if best != "" {
			// 并列时优先锚定值
			if anchorVal != "" && weights[anchorVal] == bestW {
				return anchorVal
			}
			// 其次按 Top 顺序选择第一个达到 bestW 的值
			for _, r := range top {
				v := get(r.Loc)
				if v != "" && weights[v] == bestW {
					return v
				}
			}
			return best
		}
		if len(top) > 0 {
			return get(top[0].Loc)
		}
		return ""
	}
	out.Country = pick(func(l fusion.Location) string { return l.Country }, anchor.Loc.Country)
	out.Region = pick(func(l fusion.Location) string { return l.Region }, anchor.Loc.Region)
	out.Province = pick(func(l fusion.Location) string { return l.Province }, anchor.Loc.Province)
	out.City = pick(func(l fusion.Location) string { return l.City }, anchor.Loc.City)
	out.ISP = pick(func(l fusion.Location) string { return l.ISP }, anchor.Loc.ISP)
	// 国家兜底：当区域/城市显然属于中国而国家非中国，修正为中国
	if fusion.CoherenceCoeff(out) < 1.0 {
		logger.L().Info("fusion_country_fallback_applied", "prev_country", out.Country, "region", out.Region, "city", out.City)
		out.Country = "中国"
	}
	var maxScore, maxConf float64
	var topW *Weighted
	if len(results) > 0 {
		maxScore = results[0].Score
		maxConf = results[0].Conf
		topW = &Weighted{Loc: results[0].Loc, Score: results[0].Score, Confidence: results[0].Conf, Assoc: results[0].Assoc, Name: results[0].Name}
	}
	if topW != nil {
		logger.L().Debug("plugin_aggregate_top", "score", topW.Score, "assoc", topW.Assoc, "conf", topW.Confidence)
	}
	logger.L().Debug("plugin_aggregate_end", "ip", ip, "score", maxScore)
	return out, maxScore, maxConf, topW
}

// 文档注释：质量系数估算（与融合层一致）
// 背景：用于 Top 来源选择时的分数计算复用；不代表绝对质量。
func qualityCoeff(l fusion.Location) float64 {
	c := 0.0
	if l.Country != "" {
		c += 0.2
	}
	if l.Region != "" {
		c += 0.2
	}
	if l.Province != "" {
		c += 0.3
	}
	if l.City != "" {
		c += 0.3
	}
	return c
}

// 文档注释：读取权重（环境变量）
// 背景：允许通过环境变量微调各插件权重，上限 10。
func readWeight(env string, def float64) float64 {
	s := os.Getenv(env)
	if s == "" {
		return def
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return def
	}
	return f
}
