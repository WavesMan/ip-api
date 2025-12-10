package api

import (
    "net/http"
    "strings"
)

// 文档注释：获取客户端 IP（用于业务查询参数）
// 背景：多层代理环境下，优先显式参数，其次常见反向代理头，最后回退远端地址；确保在复杂链路中得到稳定来源 IP。
// 约束：不解析 IPv6 压缩形式的特殊头部变体；当头部存在伪造风险时需结合可信代理白名单处理。
func getClientIP(r *http.Request) string {
    q := r.URL.Query().Get("ip")
    if q != "" {
        return q
    }
    h := r.Header
    if x := h.Get("x-forwarded-for"); x != "" { return strings.Split(x, ",")[0] }
    if x := h.Get("cf-connecting-ip"); x != "" { return x }
    if x := h.Get("x-real-ip"); x != "" { return x }
    if x := h.Get("x-client-ip"); x != "" { return x }
    if x := h.Get("x-edge-client-ip"); x != "" { return x }
    if x := h.Get("x-edgeone-ip"); x != "" { return x }
    if x := h.Get("forwarded"); x != "" {
        i := strings.Index(strings.ToLower(x), "for=")
        if i >= 0 {
            y := x[i+4:]
            y = strings.Trim(y, "\" ")
            if p := strings.IndexByte(y, ';'); p >= 0 { y = y[:p] }
            if p := strings.IndexByte(y, ','); p >= 0 { y = y[:p] }
            return y
        }
    }
    host := r.RemoteAddr
    if host != "" {
        if i := strings.LastIndex(host, ":"); i > 0 { return host[:i] }
        return host
    }
    return ""
}

// 文档注释：获取访问者 IP（用于去重与限流）
// 背景：与 getClientIP 分离，避免查询目标与访问来源混淆导致去重不准；用于布隆去重键的组成。
// 约束：同样依赖常见代理头顺序；部署于未经信任的代理链路需配合网关过滤与鉴权策略。
func getVisitorIP(r *http.Request) string {
    h := r.Header
    if x := h.Get("x-forwarded-for"); x != "" { return strings.Split(x, ",")[0] }
    if x := h.Get("cf-connecting-ip"); x != "" { return x }
    if x := h.Get("x-real-ip"); x != "" { return x }
    if x := h.Get("x-client-ip"); x != "" { return x }
    if x := h.Get("x-edge-client-ip"); x != "" { return x }
    if x := h.Get("x-edgeone-ip"); x != "" { return x }
    if x := h.Get("forwarded"); x != "" {
        i := strings.Index(strings.ToLower(x), "for=")
        if i >= 0 {
            y := x[i+4:]
            y = strings.Trim(y, "\" ")
            if p := strings.IndexByte(y, ';'); p >= 0 { y = y[:p] }
            if p := strings.IndexByte(y, ','); p >= 0 { y = y[:p] }
            return y
        }
    }
    host := r.RemoteAddr
    if host != "" {
        if i := strings.LastIndex(host, ":"); i > 0 { return host[:i] }
        return host
    }
    return ""
}

