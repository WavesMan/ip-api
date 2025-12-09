package amap

import (
	"context"
	"encoding/json"
	"errors"
	"ip-api/internal/logger"
	"ip-api/internal/metrics"
	"net/http"
	"net/url"
	"time"
)

// 文档注释：高德 IP 定位响应结构
// 背景：对齐高德 REST API 的返回字段，仅解析本方案需要的省/市/编码等信息；用于离线融合与入库。
// 约束：status/infocode 用于错误判定与分类聚合；不在此处扩展对外响应模型。
type IPResponse struct {
	Status    string `json:"status"`
	Info      string `json:"info"`
	Infocode  string `json:"infocode"`
	Province  string `json:"province"`
	City      string `json:"city"`
	Adcode    string `json:"adcode"`
	Rectangle string `json:"rectangle"`
}

// 文档注释：查询单个 IP 的定位信息（REST）
// 为什么：离线采集阶段调用外部数据源，补充城市级信息用于融合入库；与在线查询链路解耦，避免引入外部不确定性。
// 参数：
// - ctx：请求上下文，用于控制超时与取消；
// - client：HTTP 客户端，可传入共享实例；为空时使用 5s 超时的默认客户端；
// - key：高德 Web 服务 API 的后端密钥，必填；
// - ip：目标 IPv4 文本；为空时由高德按来源定位，不推荐在离线作业使用。
// 返回：解析后的响应结构；当 status!="1" 时返回错误并附带响应内容以便上层记录。
// 约束：仅支持国内 IPv4；错误与无数据由上层统一降级处理。
func QueryIP(ctx context.Context, client *http.Client, key string, ip string) (*IPResponse, error) {
	if key == "" {
		return nil, errors.New("missing key")
	}
	q := url.Values{}
	q.Set("key", key)
	if ip != "" {
		q.Set("ip", ip)
	}
	u := "https://restapi.amap.com/v3/ip?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	t0 := time.Now()
	metrics.AMapRequestsTotal.Inc()
	logger.L().Debug("amap_req", "ip", ip)
	resp, err := client.Do(req)
	if err != nil {
		logger.L().Error("amap_http_error", "err", err)
		metrics.AMapFailTotal.Inc()
		return nil, err
	}
	defer resp.Body.Close()
	var r IPResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		logger.L().Error("amap_decode_error", "err", err)
		metrics.AMapFailTotal.Inc()
		return nil, err
	}
	dur := time.Since(t0).Milliseconds()
	metrics.AMapDurationMs.Observe(float64(dur))
	logger.L().Debug("amap_resp", "ip", ip, "status", r.Status, "infocode", r.Infocode, "province", r.Province, "city", r.City, "duration_ms", dur)
	if r.Status != "1" {
		metrics.AMapFailTotal.Inc()
		return &r, errors.New("amap error")
	}
	metrics.AMapSuccessTotal.Inc()
	return &r, nil
}
