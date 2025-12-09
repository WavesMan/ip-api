package middleware

import (
    "net/http"
    "os"
    "strconv"
    "sync"
    "time"
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
    if tb.tokens > 0 { tb.tokens--; return true }
    return false
}

func Wrap(next http.Handler) http.Handler {
    enabled := os.Getenv("RATE_LIMIT_ENABLED") == "true"
    if !enabled { return next }
    qps := 200
    if s := os.Getenv("RATE_LIMIT_QPS"); s != "" { if n, e := strconv.Atoi(s); e == nil && n > 0 { qps = n } }
    tb := &TokenBucket{ capacity: qps, tokens: qps, lastSec: time.Now().Unix() }
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !tb.allow() {
            w.WriteHeader(http.StatusTooManyRequests)
            return
        }
        next.ServeHTTP(w, r)
    })
}

