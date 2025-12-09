package fusion

import (
	"context"
	"ip-api/internal/amap"
	"ip-api/internal/localdb"
	"ip-api/internal/store"
	"math"
	"net/http"
	"os"
	"strconv"
)

type Location struct {
	Country  string
	Region   string
	Province string
	City     string
	ISP      string
}

type WeightedResult struct {
	Location   Location
	Score      float64
	Source     string
	Confidence float64
}

type DataSource interface {
	Query(ctx context.Context, ip string) (Location, float64)
	GetWeight() float64
	Name() string
}

// 文档注释：质量系数估算（简化）
// 背景：字段完整度作为质量近似；用于加权计算，不代表绝对置信度。
func qualityCoeff(loc Location) float64 {
	c := 0.0
	if loc.Country != "" {
		c += 0.2
	}
	if loc.Region != "" {
		c += 0.2
	}
	if loc.Province != "" {
		c += 0.3
	}
	if loc.City != "" {
		c += 0.3
	}
	return c
}

// 文档注释：加权聚合并返回融合结果与最高分
func Aggregate(ctx context.Context, sources []DataSource, ip string) (Location, float64, float64) {
	var results []WeightedResult
	for _, s := range sources {
		loc, conf := s.Query(ctx, ip)
		w := s.GetWeight()
		if w > 10 {
			w = 10
		}
		q := qualityCoeff(loc)
		score := 100 * (w / 10.0) * q * conf
		results = append(results, WeightedResult{Location: loc, Score: score, Source: s.Name(), Confidence: conf})
	}
	// 排序选前 3，多数投票（字段级）
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	top := results
	if len(top) > 3 {
		top = top[:3]
	}
	var out Location
	pick := func(get func(Location) string) string {
		m := map[string]int{}
		for _, r := range top {
			v := get(r.Location)
			m[v]++
		}
		best := ""
		bestN := -1
		for k, v := range m {
			if v > bestN && k != "" {
				best = k
				bestN = v
			}
		}
		if best != "" {
			return best
		}
		if len(top) > 0 {
			return get(top[0].Location)
		}
		return ""
	}
	out.Country = pick(func(l Location) string { return l.Country })
	out.Region = pick(func(l Location) string { return l.Region })
	out.Province = pick(func(l Location) string { return l.Province })
	out.City = pick(func(l Location) string { return l.City })
	out.ISP = pick(func(l Location) string { return l.ISP })
	maxScore := 0.0
	maxConf := 0.0
	if len(results) > 0 {
		maxScore = results[0].Score
		maxConf = results[0].Confidence
	}
	return out, maxScore, maxConf
}

// 数据源实现：AMap REST
type AMapSource struct {
	Key    string
	Client *http.Client
}

func (s *AMapSource) Query(ctx context.Context, ip string) (Location, float64) {
	var out Location
	if s.Key == "" {
		return out, 0
	}
	r, err := amap.QueryIP(ctx, s.Client, s.Key, ip)
	if err != nil || r == nil || r.Status != "1" {
		return out, 0.2
	}
	out.Country = "中国"
	out.Region = r.Province
	out.Province = r.Province
	out.City = r.City
	return out, 0.8
}
func (s *AMapSource) GetWeight() float64 { return readWeight("FUSION_WEIGHT_AMAP", 8.0) }
func (s *AMapSource) Name() string       { return "amap" }

// 数据源实现：Overrides KV（最高优先）
type KVSource struct{ Store *store.Store }

func (s *KVSource) Query(ctx context.Context, ip string) (Location, float64) {
	var out Location
	if s.Store == nil {
		return out, 0
	}
	kv, _ := s.Store.LookupKV(ctx, ip)
	if kv == nil {
		return out, 0.0
	}
	out.Country = kv.Country
	out.Region = kv.Region
	out.Province = kv.Province
	out.City = kv.City
	out.ISP = kv.ISP
	return out, 1.0
}
func (s *KVSource) GetWeight() float64 { return readWeight("FUSION_WEIGHT_KV", 10.0) }
func (s *KVSource) Name() string       { return "kv" }

// 数据源实现：IPIP 本地树
type IPIPSource struct {
	Cache interface {
		Lookup(string) (localdb.Location, bool)
	}
}

func (s *IPIPSource) Query(ctx context.Context, ip string) (Location, float64) {
	var out Location
	if s.Cache == nil {
		return out, 0
	}
	l, ok := s.Cache.Lookup(ip)
	if !ok {
		return out, 0.0
	}
	out.Country = l.Country
	out.Region = l.Region
	out.Province = l.Province
	out.City = l.City
	out.ISP = l.ISP
	conf := 0.7
	if out.City == "" {
		conf = 0.5
	}
	return out, conf
}
func (s *IPIPSource) GetWeight() float64 { return readWeight("FUSION_WEIGHT_IPIP", 5.0) }
func (s *IPIPSource) Name() string       { return "ipip" }

// 数据源实现：IP2Region v4（按需）
type IP2RSource struct {
	Cache interface {
		Lookup(string) (localdb.Location, bool)
	}
}

func (s *IP2RSource) Query(ctx context.Context, ip string) (Location, float64) {
	var out Location
	if s.Cache == nil {
		return out, 0
	}
	l, ok := s.Cache.Lookup(ip)
	if !ok {
		return out, 0.0
	}
	out.Country = l.Country
	out.Region = l.Region
	out.Province = l.Province
	out.City = l.City
	out.ISP = l.ISP
	conf := 0.6
	if out.City == "" {
		conf = 0.5
	}
	return out, conf
}
func (s *IP2RSource) GetWeight() float64 { return readWeight("FUSION_WEIGHT_IP2R", 5.0) }
func (s *IP2RSource) Name() string       { return "ip2region" }

// 读取权重的辅助函数
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

// 文档注释：根据最高分决定写库策略（简化）
// 背景：单 IP 融合后，根据分数阈值决定写入 KV（覆盖）或 Exact（精确）；CIDR 聚合后续扩展。
func DecideWrite(out Location, score float64) (writeKV bool, writeExact bool) {
	writeKV = true
	writeExact = score >= 80.0
	return
}
