package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	RequestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ipapi_requests_total",
		Help: "Total number of /api/ip requests",
	})
	RequestDurationMs = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "ipapi_request_duration_ms",
		Help:    "Request duration in milliseconds",
		Buckets: []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000},
	})
	EmptyResultsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ipapi_empty_results_total",
		Help: "Total number of responses with empty location",
	})
	RedisHitsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ipapi_redis_hits_total",
		Help: "Total redis cache hits",
	})
	RedisMissesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ipapi_redis_misses_total",
		Help: "Total redis cache misses",
	})
	AMapRequestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ipapi_amap_requests_total",
		Help: "Total amap REST requests",
	})
	AMapSuccessTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ipapi_amap_success_total",
		Help: "Total amap REST successes",
	})
	AMapFailTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ipapi_amap_fail_total",
		Help: "Total amap REST failures",
	})
	AMapDurationMs = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "ipapi_amap_duration_ms",
		Help:    "AMap REST call duration in milliseconds",
		Buckets: []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000},
	})
	PluginRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ipapi_plugin_requests_total",
		Help: "Total plugin Query requests",
	}, []string{"plugin"})
	PluginSuccessTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ipapi_plugin_success_total",
		Help: "Total plugin Query successes (non-empty result)",
	}, []string{"plugin"})
	PluginFailTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ipapi_plugin_fail_total",
		Help: "Total plugin Query failures (empty or error)",
	}, []string{"plugin"})
	PluginDurationMs = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ipapi_plugin_duration_ms",
		Help:    "Plugin Query duration in milliseconds",
		Buckets: []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000},
	}, []string{"plugin"})
	PluginHeartbeatTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ipapi_plugin_heartbeat_total",
		Help: "Plugin heartbeat count by status",
	}, []string{"plugin", "status"})
	PluginScore = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ipapi_plugin_score",
		Help:    "Plugin weighted score distribution",
		Buckets: []float64{10, 20, 40, 60, 80, 90, 100},
	}, []string{"plugin"})
)

func init() {
	prometheus.MustRegister(RequestsTotal)
	prometheus.MustRegister(RequestDurationMs)
	prometheus.MustRegister(EmptyResultsTotal)
	prometheus.MustRegister(RedisHitsTotal)
	prometheus.MustRegister(RedisMissesTotal)
	prometheus.MustRegister(AMapRequestsTotal)
	prometheus.MustRegister(AMapSuccessTotal)
	prometheus.MustRegister(AMapFailTotal)
	prometheus.MustRegister(AMapDurationMs)
	prometheus.MustRegister(PluginRequestsTotal)
	prometheus.MustRegister(PluginSuccessTotal)
	prometheus.MustRegister(PluginFailTotal)
	prometheus.MustRegister(PluginDurationMs)
	prometheus.MustRegister(PluginHeartbeatTotal)
	prometheus.MustRegister(PluginScore)
}

// 文档注释：返回 Prometheus 指标监听器
// 背景：统一暴露注册指标到 /metrics 路径，供 Prometheus 抓取；在主入口挂载。
func Handler() http.Handler { return promhttp.Handler() }
