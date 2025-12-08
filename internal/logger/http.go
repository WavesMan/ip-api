package logger

import (
  "log/slog"
  "net/http"
  "time"
)

type statusWriter struct {
  http.ResponseWriter
  status int
  bytes int
}

func (w *statusWriter) WriteHeader(code int) {
  w.status = code
  w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
  n, err := w.ResponseWriter.Write(b)
  w.bytes += n
  return n, err
}

func AccessMiddleware(l *slog.Logger) func(http.Handler) http.Handler {
  return func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      sw := &statusWriter{ResponseWriter: w, status: 200}
      start := time.Now()
      next.ServeHTTP(sw, r)
      dur := time.Since(start)
      l.Info("http_access",
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

