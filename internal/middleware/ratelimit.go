package middleware

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"ip-api/internal/logger"
	"ip-api/internal/plugins"
	"ip-api/pkg/origindefense"
)

// 文档注释：令牌桶限流中间件（每秒）
// 背景：在流量峰值时对入口进行限速，避免缓存与数据库被过载；按环境变量开关与速率配置。
// 约束：简化实现，不做队列排队，仅丢弃并返回 429；与布隆去重配合减少重复压力。
type TokenBucket struct {
	capacity int
	tokens   int
	lastSec  int64
	mu       sync.Mutex
}

func (tb *TokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	nowSec := time.Now().Unix()
	if tb.lastSec != nowSec {
		tb.lastSec = nowSec
		tb.tokens = tb.capacity
	}
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

func Wrap(next http.Handler) http.Handler {
	od := origindefense.NewFromEnv(logger.L())
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 文档注释：EdgeOne 地理上下文注入
		// 背景：在源站防御通过后，解析 EdgeOne 改写的请求头并注入到上下文，供融合层插件读取；解析失败不阻断主流程。
		// 约束：字段名大小写需与控制台配置一致；经纬度/ASN 做空值与类型容错。
		geo := parseEdgeOneGeo(r)
		logger.L().Debug("edgeone_geo_inject",
			"ip", geo.ClientIP,
			"country", geo.CountryName,
			"region", geo.RegionName,
			"province", "",
			"city", geo.CityName,
			"isp", geo.ISP,
			"lat", geo.Latitude,
			"lon", geo.Longitude,
			"asn", geo.ASN,
		)
		ctx := context.WithValue(r.Context(), "edgeone_geo", geo)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
	h := od.Wrap(inner)
	if os.Getenv("RATE_LIMIT_ENABLED") == "true" {
		qps := 200
		if s := os.Getenv("RATE_LIMIT_QPS"); s != "" {
			if n, e := strconv.Atoi(s); e == nil && n > 0 {
				qps = n
			}
		}
		tb := &TokenBucket{capacity: qps, tokens: qps, lastSec: time.Now().Unix()}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !tb.allow() {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			h.ServeHTTP(w, r)
		})
	}
	return h
}

// 文档注释：解析 EdgeOne 请求头为地理信息结构
// 背景：读取自定义头中的国家/地区/城市/运营商等字段，转换为标准结构体传递到后续处理；不做外部依赖调用。
// 约束：仅进行基础的字符串读取与数值转换；异常值将被忽略。
func parseEdgeOneGeo(r *http.Request) plugins.EdgeOneGeoInfo {
	h := r.Header
	var g plugins.EdgeOneGeoInfo
	g.CountryName = h.Get("X-EO-Geo-Country")
	g.CountryCodeAlpha2 = h.Get("X-EO-Geo-CountryCodeAlpha2")
	g.CountryCodeAlpha3 = h.Get("X-EO-Geo-CountryCodeAlpha3")
	g.RegionName = h.Get("X-EO-Geo-Region")
	g.RegionCode = h.Get("X-EO-Geo-RegionCode")
	g.CityName = h.Get("X-EO-Geo-City")
	// 优先新版头部 X-EO-ISP，兼容旧名 X-EO-Geo-CISP
	if v := h.Get("X-EO-ISP"); v != "" {
		g.ISP = v
	} else {
		g.ISP = h.Get("X-EO-Geo-CISP")
	}
	g.ClientIP = h.Get("X-EO-Client-IP")
	if s := h.Get("X-EO-Geo-Latitude"); s != "" {
		if v, e := strconv.ParseFloat(s, 64); e == nil {
			g.Latitude = v
		}
	}
	if s := h.Get("X-EO-Geo-Longitude"); s != "" {
		if v, e := strconv.ParseFloat(s, 64); e == nil {
			g.Longitude = v
		}
	}
	if s := h.Get("X-EO-Geo-ASN"); s != "" {
		if v, e := strconv.Atoi(s); e == nil {
			g.ASN = v
		}
	}
	logger.L().Debug("edgeone_geo_parse",
		"ip", g.ClientIP,
		"country", g.CountryName,
		"region", g.RegionName,
		"city", g.CityName,
		"isp", g.ISP,
		"lat", g.Latitude,
		"lon", g.Longitude,
		"asn", g.ASN,
	)
	return g
}
