package origindefense

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// 文档注释：源站防御（IP/CIDR白名单 + TEO 回源网段）
// 背景：作为源站部署在 EdgeOne CDN 后，仅允许 EdgeOne 回源 IP 与指定开发调试 IP 直接访问源站；其他请求统一返回 403。
// 约束：
// 1) 不依赖项目内部代码，提供独立包以便在其他项目直接复用；
// 2) 支持 IPv4/IPv6 CIDR；
// 3) 可选定期轮询 TEO DescribeOriginACL，若 NextOriginACL 返回则使用最新网段；
// 4) 真实来源 IP 以 RemoteAddr 为准；如需识别上游真实 IP，请通过 ORIGIN_REAL_IP_HEADER 指定。
type Middleware struct {
	l            *slog.Logger
	allowIPs     map[string]struct{}
	allowCIDRs   []*net.IPNet
	realIPHeader string
	mu           sync.RWMutex
}

// NewFromEnv：按环境变量构建中间件并启动 TEO 轮询（如配置齐备）
// 环境变量：
// ORIGIN_DEFENSE_ENABLE=true              是否启用防御
// ORIGIN_ALLOW_IPS=1.2.3.4,5.6.7.8       允许的单 IP 列表（逗号分隔）
// ORIGIN_ALLOW_CIDRS=10.0.0.0/8,...      允许的 CIDR 列表（逗号分隔，支持 v4/v6）
// ORIGIN_ALLOW_LOCAL=true                 允许 127.0.0.1/::1（本地开发）
// ORIGIN_REAL_IP_HEADER=X-Forwarded-For   指定上游真实 IP 头（首个有效 IP 生效）
// TEO_ENABLE=true                         是否启用 TEO 回源网段轮询
// TC_SECRET_ID/TC_SECRET_KEY              腾讯云 APIv3 访问凭证
// TEO_ZONE_ID                             站点 ID
// TEO_REGION                               区域（可选）
// TEO_POLL_SECONDS                        轮询周期秒（默认 259200 = 3 天）
func NewFromEnv(l *slog.Logger) *Middleware {
	m := &Middleware{l: l, allowIPs: map[string]struct{}{}, realIPHeader: strings.TrimSpace(os.Getenv("ORIGIN_REAL_IP_HEADER"))}
	// 允许单 IP
	if s := os.Getenv("ORIGIN_ALLOW_IPS"); s != "" {
		for _, p := range strings.Split(s, ",") {
			p = strings.TrimSpace(p)
			if ip := net.ParseIP(p); ip != nil {
				m.allowIPs[ip.String()] = struct{}{}
			}
		}
	}
	// 允许 CIDR
	if s := os.Getenv("ORIGIN_ALLOW_CIDRS"); s != "" {
		for _, c := range strings.Split(s, ",") {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			if _, n, err := net.ParseCIDR(c); err == nil {
				m.allowCIDRs = append(m.allowCIDRs, n)
			}
		}
	}
	// 本地允许
	if os.Getenv("ORIGIN_ALLOW_LOCAL") == "true" {
		if ip := net.ParseIP("127.0.0.1"); ip != nil {
			m.allowIPs[ip.String()] = struct{}{}
		}
		if ip := net.ParseIP("::1"); ip != nil {
			m.allowIPs[ip.String()] = struct{}{}
		}
	}
	// 启动 TEO 轮询
	if os.Getenv("TEO_ENABLE") == "true" && os.Getenv("TC_SECRET_ID") != "" && os.Getenv("TC_SECRET_KEY") != "" && os.Getenv("TEO_ZONE_ID") != "" {
		period := 259200
		if s := os.Getenv("TEO_POLL_SECONDS"); s != "" {
			if n, e := parseInt(s); e == nil && n > 0 {
				period = n
			}
		}
		go m.pollTEO(time.Duration(period) * time.Second)
	}
	return m
}

// Wrap：生成 http.Handler 中间件
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	enabled := os.Getenv("ORIGIN_DEFENSE_ENABLE") == "true"
	if !enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := m.extractIP(r)
		if ip == nil {
			m.l.Debug("origin_defense_block", "reason", "no_ip")
			write403(w)
			return
		}
		if m.allowed(ip) {
			next.ServeHTTP(w, r)
			return
		}
		m.l.Debug("origin_defense_block", "ip", ip.String())
		write403(w)
	})
}

// allowed：判断 IP 是否在允许集合
func (m *Middleware) allowed(ip net.IP) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.allowIPs[ip.String()]; ok {
		return true
	}
	for _, n := range m.allowCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// extractIP：解析请求来源 IP；优先指定头的首个有效 IP
func (m *Middleware) extractIP(r *http.Request) net.IP {
	if m.realIPHeader != "" {
		raw := r.Header.Get(m.realIPHeader)
		if raw != "" {
			// 取第一个逗号分隔的 IP
			parts := strings.Split(raw, ",")
			first := strings.TrimSpace(parts[0])
			if ip := net.ParseIP(first); ip != nil {
				return ip
			}
		}
	}
	host := r.RemoteAddr
	// RemoteAddr 可能包含端口
	if strings.Contains(host, ":") {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
	}
	return net.ParseIP(host)
}

// pollTEO：定期拉取 TEO 回源网段并更新允许 CIDR 集合
func (m *Middleware) pollTEO(period time.Duration) {
	for {
		if cidrs, err := fetchTEOOriginCIDRs(); err == nil {
			v4 := cidrs.IPv4
			v6 := cidrs.IPv6
			parsed := make([]*net.IPNet, 0, len(v4)+len(v6))
			for _, c := range append(v4, v6...) {
				if _, n, err := net.ParseCIDR(c); err == nil {
					parsed = append(parsed, n)
				}
			}
			m.mu.Lock()
			m.allowCIDRs = mergeCIDRs(m.allowCIDRs, parsed)
			m.mu.Unlock()
			m.l.Info("origin_defense_teo_sync_ok", "v4", len(v4), "v6", len(v6))
		} else {
			m.l.Error("origin_defense_teo_sync_error", "err", err)
		}
		time.Sleep(period)
	}
}

// write403：返回统一 403 页面（简约风格，与前端语气一致）
func write403(w http.ResponseWriter) {
	w.Header().Set("content-type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>403 禁止访问</title><style>body{font-family:system-ui,-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;background:#0f172a;color:#e2e8f0}.card{background:#111827;border:1px solid #374151;border-radius:12px;padding:24px;max-width:560px;box-shadow:0 10px 25px rgba(0,0,0,.35)}.title{font-size:20px;font-weight:700;color:#60a5fa}.desc{margin-top:8px;color:#9ca3af}.code{margin-top:12px;font-size:14px;color:#fca5a5}</style><div class="card"><div class="title">403 禁止访问</div><div class="desc">源站仅对受信回源网段与指定调试IP开放访问。</div><div class="code">如需调试，请联系管理员将你的IP加入白名单。</div></div></html>`))
}

// mergeCIDRs：合并并去重 CIDR 列表
func mergeCIDRs(old, add []*net.IPNet) []*net.IPNet {
	m := map[string]*net.IPNet{}
	for _, n := range old {
		m[n.String()] = n
	}
	for _, n := range add {
		m[n.String()] = n
	}
	out := make([]*net.IPNet, 0, len(m))
	for _, n := range m {
		out = append(out, n)
	}
	return out
}

// parseInt：轻量整型解析（避免 strconv 引入）
func parseInt(s string) (int, error) {
	var n int
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, &json.InvalidUnmarshalError{}
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
