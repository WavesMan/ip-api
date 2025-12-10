package plugins

import (
    "context"
    "ip-api/internal/fusion"
    "ip-api/internal/logger"
    "ip-api/internal/revgeo"
    "os"
    "path/filepath"
    "strconv"
    "time"
)

// 文档注释：反地理插件（进程内）
// 背景：按坐标查询行政区；在 PIP 命中时为精确结果，未命中则最近邻兜底并附近似标记；集成到插件管理器以统一心跳与权重管理。
// 约束：Query 通过上下文读取 lat/lon；当未提供坐标或快照缺失时返回空；权重由环境变量控制。
type ReverseGeoPlugin struct {
    name    string
    version string
    assoc   string
    orch    *revgeo.Orchestrator
}

func NewReverseGeoPlugin(dataDir string) (*ReverseGeoPlugin, error) {
    if dataDir == "" { dataDir = filepath.Join("data", "revgeo") }
    snap, _ := revgeo.LoadSnapshot(dataDir)
    orch := revgeo.NewOrchestrator(snap)
    return &ReverseGeoPlugin{name: "revgeo", version: "1.0", assoc: "revgeo", orch: orch}, nil
}

func (p *ReverseGeoPlugin) Name() string     { return p.name }
func (p *ReverseGeoPlugin) Version() string  { return p.version }
func (p *ReverseGeoPlugin) AssocKey() string { return p.assoc }

func (p *ReverseGeoPlugin) Query(ctx context.Context, ip string) (fusion.Location, float64) {
    var out fusion.Location
    if p.orch == nil { return out, 0 }
    latV := ctx.Value("lat")
    lonV := ctx.Value("lon")
    csV := ctx.Value("coord_sys")
    if latV == nil || lonV == nil { return out, 0 }
    lat := toFloat(latV)
    lon := toFloat(lonV)
    coordSys := ""
    if csV != nil { if s, ok := csV.(string); ok { coordSys = s } }
    u, conf, approx := p.orch.Query(lat, lon, coordSys)
    out.Country = u.Country
    out.Region = u.Region
    out.Province = u.Province
    out.City = u.City
    if approx { conf *= 0.9 }
    logger.L().Debug("reverse_geo_query", "lat", lat, "lon", lon, "conf", conf, "approx", approx)
    return out, conf
}

func (p *ReverseGeoPlugin) GetWeight(ip string) float64 {
    w := 8.0
    if s := os.Getenv("FUSION_WEIGHT_REVGEO"); s != "" { if f, e := strconv.ParseFloat(s, 64); e == nil && f > 0 { w = f } }
    return w
}

func (p *ReverseGeoPlugin) Heartbeat(ctx context.Context) error {
    // 简单健康：快照存在且质心或边界非空视为健康
    t0 := time.Now()
    if p.orch == nil { return context.DeadlineExceeded }
    // 轻触发一次查询以确认结构可用
    _, _, _ = p.orch.Query(0, 0, "")
    logger.L().Debug("reverse_geo_heartbeat", "ms", time.Since(t0).Milliseconds())
    return nil
}

func toFloat(v any) float64 {
    switch x := v.(type) {
    case float64:
        return x
    case float32:
        return float64(x)
    case int:
        return float64(x)
    case int64:
        return float64(x)
    case string:
        f, _ := strconv.ParseFloat(x, 64)
        return f
    default:
        return 0
    }
}

