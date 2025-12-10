package api

import (
    "context"
    "hash/fnv"
    "time"

    "github.com/redis/go-redis/v9"
)

// 文档注释：计算布隆过滤器位置
// 参数：data 为参与哈希的字节序列，m 为位图大小（建议 2 的幂以便分布更均匀），k 为哈希次数（控制误判率与写入开销）。
// 背景：使用 FNV64a 结合索引扰动生成 k 个位置，用于 GetBit/SetBit；适配短周期去重场景。
// 约束：m、k 需结合实际 QPS 与 TTL 调参，避免过高误判率或写入开销过大。
func bloomPositions(data []byte, m uint32, k int) []int64 {
    pos := make([]int64, k)
    for i := 0; i < k; i++ {
        h := fnv.New64a()
        h.Write([]byte{byte(i)})
        h.Write(data)
        v := h.Sum64()
        p := uint32(v % uint64(m))
        pos[i] = int64(p)
    }
    return pos
}

// 文档注释：检查并写入布隆过滤器位图
// 背景：用于短周期去重，降低重复请求对缓存与后端的压力；命中视为“已见过”，不再重复处理。
// 返回：true 表示首次见到（已写入位图，可继续处理）；false 表示已存在（建议直接快速返回或限频）。
// 异常：Redis 交互错误时返回 error；当 rc 为 nil 时视为“允许处理”，避免阻断主流程。
func bloomCheckAndSet(ctx context.Context, rc *redis.Client, key string, positions []int64, ttl time.Duration) (bool, error) {
    if rc == nil { return true, nil }
    seen := true
    for _, p := range positions {
        b, err := rc.GetBit(ctx, key, p).Result()
        if err != nil { return true, err }
        if b == 0 { seen = false }
    }
    if !seen {
        for _, p := range positions { _, _ = rc.SetBit(ctx, key, p, 1).Result() }
        _ = rc.Expire(ctx, key, ttl).Err()
        return true, nil
    }
    return false, nil
}

