// 包 logger：统一初始化与获取日志器，避免各模块重复配置；通过环境变量控制日志级别与输出格式
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// 默认日志器：在进程级复用，避免多处初始化导致输出不一致
var defaultLogger *slog.Logger

// Setup：初始化默认日志器
// 背景：集中化日志配置，便于按环境统一调整级别与格式
// 约束：输出目标固定为标准错误；不在此处管理文件句柄或外部聚合通道
func Setup() *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	format := strings.ToLower(os.Getenv("LOG_FORMAT"))
	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	} else {
		h = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	}
	defaultLogger = slog.New(h)
	return defaultLogger
}

// L：获取默认日志器
// 背景：为业务代码提供快捷访问；若未初始化则回退到 Setup
func L() *slog.Logger {
	if defaultLogger == nil {
		return Setup()
	}
	return defaultLogger
}
