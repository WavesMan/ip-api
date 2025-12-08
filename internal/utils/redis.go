// 包 utils：Redis 连接工具，统一环境变量读取与可选 DB 选择
package utils

import (
	"ip-api/internal/logger"
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// OpenRedis：使用地址与密码打开 Redis 客户端
// 背景：保留直接传入参数的能力，用于测试与手工注入场景
func OpenRedis(addr, pass string) *redis.Client {
	if addr == "" {
		return nil
	}
	return redis.NewClient(&redis.Options{Addr: addr, Password: pass})
}

// OpenRedisFromEnv：从环境变量打开 Redis 客户端，支持 REDIS_DB 选择
// 约束：REDIS_DB 解析失败时忽略并回退到 0；未配置地址时返回 nil
func OpenRedisFromEnv() *redis.Client {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}
	addr := host + ":" + port
	pass := os.Getenv("REDIS_PASS")
	db := 0
	if v := os.Getenv("REDIS_DB"); v != "" {
		// ignore parse error silently, default 0
		if n, _ := strconv.Atoi(v); n >= 0 {
			db = n
		}
	}
	logger.L().Debug("redis_env", "addr", addr, "db", db)
	return redis.NewClient(&redis.Options{Addr: addr, Password: pass, DB: db})
}
