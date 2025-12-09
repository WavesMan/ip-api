package plugins

import (
	"context"
	"encoding/json"
	"ip-api/internal/fusion"
	"net/http"
	"time"
)

// 文档注释：外部 HTTP 插件适配器
// 背景：为不可信或第三方数据源提供进程外接入方式，通过简单 HTTP 契约实现查询与心跳。
// 约束：约定 /health 与 /query?ip= 接口；响应结构需包含归一化字段与可选置信度；主服务设置超时与错误降级。
type HTTPPlugin struct {
	name     string
	version  string
	assoc    string
	endpoint string
	weight   float64
	client   *http.Client
}

func NewHTTP(name, version, assoc, endpoint string, weight float64) *HTTPPlugin {
	return &HTTPPlugin{name: name, version: version, assoc: assoc, endpoint: endpoint, weight: weight, client: &http.Client{Timeout: 3 * time.Second}}
}

func (h *HTTPPlugin) Name() string                { return h.name }
func (h *HTTPPlugin) Version() string             { return h.version }
func (h *HTTPPlugin) AssocKey() string            { return h.assoc }
func (h *HTTPPlugin) GetWeight(ip string) float64 { return h.weight }

// 文档注释：心跳检测
// 背景：访问 /health 用于探测可用性；非 200 视为不可用以便熔断。
func (h *HTTPPlugin) Heartbeat(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, h.endpoint+"/health", nil)
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return context.DeadlineExceeded
	}
	return nil
}

// 文档注释：查询接口
// 背景：调用 /query?ip= 获取归一化 Location 与置信度；异常降级为低置信度或空结果。
func (h *HTTPPlugin) Query(ctx context.Context, ip string) (fusion.Location, float64) {
	var out fusion.Location
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, h.endpoint+"/query?ip="+ip, nil)
	resp, err := h.client.Do(req)
	if err != nil {
		return out, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out, 0
	}
	var m struct {
		Country    string  `json:"country"`
		Region     string  `json:"region"`
		Province   string  `json:"province"`
		City       string  `json:"city"`
		ISP        string  `json:"isp"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return out, 0
	}
	out.Country = m.Country
	out.Region = m.Region
	out.Province = m.Province
	out.City = m.City
	out.ISP = m.ISP
	if m.Confidence <= 0 {
		m.Confidence = 0.5
	}
	return out, m.Confidence
}
