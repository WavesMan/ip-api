package plugins

import (
	"context"
	"ip-api/internal/fusion"
	"ip-api/internal/localdb"
)

// 文档注释：IP2Region 插件（进程内）
// 背景：基于 v4 XDB 本地库查询；作为融合的离线数据源补充。
// 约束：仅处理 IPv4；城市缺失时置信度降低；权重默认由环境变量微调。
type IP2RegionPlugin struct {
	cache interface {
		Lookup(string) (localdb.Location, bool)
	}
}

func NewIP2RegionPlugin(cache interface {
	Lookup(string) (localdb.Location, bool)
}) *IP2RegionPlugin {
	return &IP2RegionPlugin{cache: cache}
}

func (p *IP2RegionPlugin) Name() string     { return "ip2region" }
func (p *IP2RegionPlugin) Version() string  { return "1.0" }
func (p *IP2RegionPlugin) AssocKey() string { return "ip2r" }

func (p *IP2RegionPlugin) Query(ctx context.Context, ip string) (fusion.Location, float64) {
	var out fusion.Location
	if p.cache == nil {
		return out, 0
	}
	l, ok := p.cache.Lookup(ip)
	if !ok {
		return out, 0
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

func (p *IP2RegionPlugin) GetWeight(ip string) float64         { return readWeight("FUSION_WEIGHT_IP2R", 5.0) }
func (p *IP2RegionPlugin) Heartbeat(ctx context.Context) error { return nil }
