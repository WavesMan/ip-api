package utils

import (
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"
)

func OpenRedis(addr, pass string) *redis.Client {
	if addr == "" {
		return nil
	}
	return redis.NewClient(&redis.Options{Addr: addr, Password: pass})
}

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
	return redis.NewClient(&redis.Options{Addr: addr, Password: pass, DB: db})
}
