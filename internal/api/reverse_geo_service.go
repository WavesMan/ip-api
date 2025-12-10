package api

import (
	"context"
	"encoding/json"
	"fmt"
	"ip-api/internal/logger"
	"ip-api/internal/metrics"
	"ip-api/internal/plugins"
	"time"

	"github.com/redis/go-redis/v9"
)

// 文档注释：内部反地理查询函数（供代码调用）
// 背景：替代对外 HTTP 端点，供服务内部按坐标查询行政区；保留进程内缓存与融合逻辑。
// 参数：lat/lon（WGS84或声明的坐标系），coordSys 可为空；pm 插件管理器，rc 可选用于热点缓存。
// 返回：统一的 Location 与置信度、近似标记；错误仅限解析/上下文取消。
type ReverseGeoResult struct {
	Country    string
	Region     string
	Province   string
	City       string
	Confidence float64
	Approx     bool
}

func ReverseGeoQuery(ctx context.Context, rc *redis.Client, pm *plugins.Manager, lat float64, lon float64, coordSys string, cacheTTLSeconds int) (*ReverseGeoResult, error) {
	tBegin := time.Now()
	metrics.ReverseGeoRequestsTotal.Inc()
	key := "revgeo:" + formatCoord(lat) + ":" + formatCoord(lon)
	var out ReverseGeoResult
	if rc != nil {
		if s, _ := rc.Get(ctx, key).Result(); s != "" {
			_ = json.Unmarshal([]byte(s), &out)
			metrics.ReverseGeoDurationMs.Observe(float64(time.Since(tBegin).Milliseconds()))
			return &out, nil
		}
	}
	ctx2 := context.WithValue(ctx, "lat", lat)
	ctx2 = context.WithValue(ctx2, "lon", lon)
	if coordSys != "" {
		ctx2 = context.WithValue(ctx2, "coord_sys", coordSys)
	}
	loc, _, conf, top := pm.Aggregate(ctx2, "")
	approx := false
	if top != nil && top.Name == "revgeo" && conf < 0.8 {
		approx = true
	}
	out.Country = loc.Country
	out.Region = loc.Region
	out.Province = loc.Province
	out.City = loc.City
	out.Confidence = conf
	out.Approx = approx
	logger.L().Debug("reverse_geo_internal", "lat", lat, "lon", lon, "conf", conf, "approx", approx)
	if rc != nil {
		b, _ := json.Marshal(out)
		ttl := time.Duration(cacheTTLSeconds) * time.Second
		if cacheTTLSeconds <= 0 {
			ttl = 3600 * time.Second
		}
		_ = rc.Set(ctx, key, string(b), ttl).Err()
	}
	metrics.ReverseGeoDurationMs.Observe(float64(time.Since(tBegin).Milliseconds()))
	return &out, nil
}

func formatCoord(v float64) string { return strconvFormatFloat(v, 3) }

func strconvFormatFloat(f float64, prec int) string {
	// 避免引入 strconv 依赖在此文件重复导入，保持局部实现
	// 简化为固定精度格式化，足以满足缓存键稳定性
	s := fmt.Sprintf("%.3f", f)
	return s
}
