// 包 api：集中注册 HTTP API 路由以解耦主入口，便于后续扩展与替换
package api

import (
	"encoding/json"
	"ip-api/internal/store"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// 查询结果结构：仅包含对外返回必要字段
type queryResult struct {
	IP       string `json:"ip"`
	Country  string `json:"country"`
	Region   string `json:"region"`
	Province string `json:"province"`
	City     string `json:"city"`
	ISP      string `json:"isp"`
}

// 解析访问者 IP：优先参数，其次常见反向代理头；保证在多层代理场景下稳定获取源 IP
func getClientIP(r *http.Request) string {
	q := r.URL.Query().Get("ip")
	if q != "" {
		return q
	}
	h := r.Header
	if x := h.Get("x-forwarded-for"); x != "" {
		return strings.Split(x, ",")[0]
	}
	if x := h.Get("cf-connecting-ip"); x != "" {
		return x
	}
	if x := h.Get("x-real-ip"); x != "" {
		return x
	}
	if x := h.Get("x-client-ip"); x != "" {
		return x
	}
	if x := h.Get("x-edge-client-ip"); x != "" {
		return x
	}
	if x := h.Get("x-edgeone-ip"); x != "" {
		return x
	}
	if x := h.Get("forwarded"); x != "" {
		i := strings.Index(strings.ToLower(x), "for=")
		if i >= 0 {
			y := x[i+4:]
			y = strings.Trim(y, "\" ")
			if p := strings.IndexByte(y, ';'); p >= 0 {
				y = y[:p]
			}
			if p := strings.IndexByte(y, ','); p >= 0 {
				y = y[:p]
			}
			return y
		}
	}
	return ""
}

// 构建并返回 API 路由：独立 ServeMux 便于在主入口挂载到 /api 前缀
func BuildRoutes(st *store.Store, rc *redis.Client) *http.ServeMux {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/ip", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ip := r.URL.Query().Get("ip")
		if ip == "" {
			ip = getClientIP(r)
		}
		var res queryResult
		res.IP = ip
		if ip != "" && rc != nil {
			s, _ := rc.Get(ctx, "ip:"+ip).Result()
			if s != "" {
				_ = json.Unmarshal([]byte(s), &res)
				w.Header().Set("content-type", "application/json; charset=utf-8")
				w.Header().Set("cache-control", "no-store")
				_ = json.NewEncoder(w).Encode(res)
				_ = st.IncrStats(ctx, ip)
				return
			}
		}
		loc, _ := st.LookupIP(ctx, ip)
		if loc != nil {
			res.Country = loc.Country
			res.Region = loc.Region
			res.Province = loc.Province
			res.City = loc.City
			res.ISP = loc.ISP
			if rc != nil {
				b, _ := json.Marshal(res)
				rc.Set(ctx, "ip:"+ip, string(b), time.Hour*24)
			}
			_ = st.IncrStats(ctx, ip)
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.Header().Set("cache-control", "no-store")
		_ = json.NewEncoder(w).Encode(res)
	})

	apiMux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		t, _ := st.GetTotals(r.Context())
		m := map[string]any{"total": t.Total, "today": t.Today}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.Header().Set("cache-control", "no-store")
		_ = json.NewEncoder(w).Encode(m)
	})

	return apiMux
}
