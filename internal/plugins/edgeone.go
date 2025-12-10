package plugins

import (
	"context"
	"ip-api/internal/fusion"
	"ip-api/internal/logger"
	"math"
	"os"
	"strconv"
)

// 文档注释：EdgeOne 地理信息载体
// 背景：承载来自 TEO/EdgeOne 改写请求头的地理数据，用于在中间件阶段解析并传递到插件融合层；不直接对外暴露。
// 约束：字段命名与 EdgeOne 控制台自定义头保持一致（大小写严格），数值类型按需转换并做空值容错。
type EdgeOneGeoInfo struct {
	CountryName       string
	CountryCodeAlpha2 string
	CountryCodeAlpha3 string
	RegionName        string
	RegionCode        string
	CityName          string
	Latitude          float64
	Longitude         float64
	ASN               int
	ISP               string
	ClientIP          string
}

// 文档注释：EdgeOne 插件（进程内）
// 背景：读取请求上下文中的 EdgeOneGeo 信息，将其映射为融合层统一字段并赋予高权重/高置信度，用于补全省市等精细字段。
// 约束：仅在 geo.ClientIP 与查询目标 IP 一致时生效；缺失或不一致返回低置信度以避免误用。
type EdgeOnePlugin struct{}

func NewEdgeOnePlugin() *EdgeOnePlugin { return &EdgeOnePlugin{} }

func (p *EdgeOnePlugin) Name() string     { return "edgeone" }
func (p *EdgeOnePlugin) Version() string  { return "1.0" }
func (p *EdgeOnePlugin) AssocKey() string { return "edgeone" }

// 文档注释：查询并映射 EdgeOne 地理信息
// 背景：从请求上下文读取中间件注入的 geo 信息，按字段完整度估算置信度并输出归一化位置；不依赖外部网络调用。
func (p *EdgeOnePlugin) Query(ctx context.Context, ip string) (fusion.Location, float64) {
	var out fusion.Location
	v := ctx.Value("edgeone_geo")
	if v == nil {
		logger.L().Debug("edgeone_plugin_ctx_missing", "ip", ip)
		return out, 0.2
	}
	g, ok := v.(EdgeOneGeoInfo)
	if !ok {
		logger.L().Debug("edgeone_plugin_ctx_type_mismatch", "ip", ip)
		return out, 0.2
	}
	if g.ClientIP != "" && g.ClientIP != ip {
		logger.L().Debug("edgeone_plugin_ip_mismatch", "ip", ip, "client_ip", g.ClientIP)
		return out, 0.2
	}
	out.Country = g.CountryName
	out.Region = g.RegionName
	out.Province = ""
	out.City = g.CityName
	out.ISP = g.ISP
	c := 0.2
	if out.City != "" {
		c = 0.9
	} else if out.Region != "" {
		c = 0.8
	} else if out.Country != "" {
		c = 0.7
	}
	logger.L().Debug("edgeone_plugin_query_ok",
		"ip", ip,
		"country", out.Country,
		"region", out.Region,
		"province", out.Province,
		"city", out.City,
		"isp", out.ISP,
		"confidence", c,
	)
	return out, c
}

// 文档注释：读取插件权重（环境变量可调）
// 背景：允许通过 `FUSION_WEIGHT_EDGEONE` 微调融合权重，默认接近满分以优先采用 EdgeOne 回传结果；上限 10。
func (p *EdgeOnePlugin) GetWeight(ip string) float64 {
	s := os.Getenv("FUSION_WEIGHT_EDGEONE")
	if s == "" {
		return 9.8
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 9.8
	}
	if f > 10 {
		f = 10
	}
	if f < 0 {
		f = 0
	}
	return f
}

// 文档注释：心跳检查（本地数据源始终健康）
// 背景：不依赖外部网络或服务，只读取请求上下文；心跳恒定为健康以参与融合。
func (p *EdgeOnePlugin) Heartbeat(ctx context.Context) error { return nil }
