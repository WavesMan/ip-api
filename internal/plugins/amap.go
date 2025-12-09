package plugins

import (
	"context"
	"ip-api/internal/amap"
	"ip-api/internal/fusion"
	"ip-api/internal/metrics"
	"net/http"
	"time"
)

// 文档注释：AMap 插件（进程内）
// 背景：通过高德 IP 定位接口进行实时查询；用于融合的在线数据源。
// 约束：需服务端密钥；接口不可用时返回低置信度；权重默认由环境变量微调在融合层计算。
type AMapPlugin struct {
	key    string
	client *http.Client
}

func NewAMapPlugin(key string, client *http.Client) *AMapPlugin {
	return &AMapPlugin{key: key, client: client}
}

func (p *AMapPlugin) Name() string     { return "amap" }
func (p *AMapPlugin) Version() string  { return "1.0" }
func (p *AMapPlugin) AssocKey() string { return "amap" }

func (p *AMapPlugin) Query(ctx context.Context, ip string) (fusion.Location, float64) {
	var out fusion.Location
	t0 := time.Now()
	metrics.AMapRequestsTotal.Inc()
	if p.key == "" {
		return out, 0
	}
	r, err := amap.QueryIP(ctx, p.client, p.key, ip)
	ms := float64(time.Since(t0).Milliseconds())
	if err != nil || r == nil || r.Status != "1" {
		metrics.AMapFailTotal.Inc()
		metrics.AMapDurationMs.Observe(ms)
		return out, 0.2
	}
	out.Country = "中国"
	out.Region = "中国"
	out.Province = r.Province
	out.City = r.City
	metrics.AMapSuccessTotal.Inc()
	metrics.AMapDurationMs.Observe(ms)
	return out, 0.8
}

func (p *AMapPlugin) GetWeight(ip string) float64         { return readWeight("FUSION_WEIGHT_AMAP", 8.0) }
func (p *AMapPlugin) Heartbeat(ctx context.Context) error { return nil }
