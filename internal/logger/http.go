// 包 logger：http访问日志中间件，统一记录外部访问的关键维度（方法、路径、状态、耗时、字节数、远端地址）
package logger

import (
	"log/slog"
	"net/http"
	"time"
)

// statusWriter：包装 ResponseWriter 以捕获状态码与写出字节数
// 背景：标准库不暴露已写状态，需中间件层统计响应信息
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

// WriteHeader：捕获状态码并透传写头
func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Write：累加写出字节数并透传写入
func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// AccessMiddleware：生成访问日志中间件
// 为什么：统一记录外部访问，便于问题排查与性能监控；不读取请求体，避免性能与隐私风险
// 约束：远端地址来源于 RemoteAddr，若存在反向代理请结合上游真实 IP 头部在业务层处理
func AccessMiddleware(l *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w, status: 200}
			start := time.Now()
			next.ServeHTTP(sw, r)
			dur := time.Since(start)
            l.Debug("http_access",
                "method", r.Method,
                "path", r.URL.Path,
                "status", sw.status,
                "bytes", sw.bytes,
                "duration_ms", dur.Milliseconds(),
                "ip", r.RemoteAddr,
            )
		})
	}
}
