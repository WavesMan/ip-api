# IP 归属地查询服务

高并发 IPv4 查询，优先本地文件缓存，未命中回退 PostgreSQL。支持 KV 覆盖快速修正（如修正 1.1.1.1 显示“保留地址”）。

**接口与能力**
- `GET /api/ip?ip=...` 返回 `country/region/province/city/isp`（自动识别客户端 IP）。实现位置：`internal/api/ip-api.go`
- `GET /api/stats` 返回总计与当日服务量。实现位置：`internal/api/ip-api.go`
- `GET /api/version` 返回 `commit` 与 `builtAt`。实现位置：`internal/api/ip-api.go:199-204`
- Redis 热点缓存（可选），TTL 可配置：`CACHE_TTL_SECONDS`。命中逻辑：`internal/api/ip-api.go:244-276,309-317`
- 去重布隆过滤器窗口：`DEDUP_TTL_SECONDS`。实现位置：`internal/api/ip-api.go:214-234,154-180`

**查询路径优先级**
- 精确文件库命中（ExactDB）→ 前缀树缓存（IPIP）→ 数据库回退（范围或特例）。组合器：`internal/localdb/multicache.go`
- DB 回退优先检查 KV 覆盖：`internal/store/store.go:62-70`，随后依次 `_ip_overrides`、`_ip_exact`、`_ip_cidr_special`
- 启动时自动构建精确文件库：如 `_ip_overrides` 或 `_ip_overrides_kv` 有数据则生成 `exact.db` 并加载。位置：`cmd/main.go:138-160`
- 精确文件库构建时合并 KV：`internal/localdb/exactdb.go:39,52-72`

**目录结构**
- 后端入口：`cmd/main.go`
- API 路由：`internal/api/ip-api.go`
- 数据库层：`internal/store/store.go`
- 本地缓存：`internal/localdb/`
- 版本信息：`internal/version/version.go`
- 前端应用：`ui/`
- KV 覆盖 CLI：`cmd/override-kv/main.go`

**环境变量（核心）**
- `ADDR` 服务地址，默认 `:8080`
- `API_BASE` API 前缀，默认 `/api`
- `UI_DIST` 前端静态目录，默认 `ui/dist`
- `PG_HOST/PG_PORT/PG_USER/PG_PASSWORD/PG_DB/PG_SSLMODE` 数据库连接
- `REDIS_*` Redis 参数（可选）
- `CACHE_TTL_SECONDS`、`DEDUP_TTL_SECONDS` 缓存与去重 TTL（秒）
- `IPIP_PATH` 本地 IPIP 数据源路径，默认 `data/ipip/ipipfree.ipdb`

**Docker 构建**
- 强制读取 `.git` 注入版本：`Dockerfile:29-36` 显式 `COPY .git .git`，构建阶段 `git rev-parse`/`git log` 自动注入 `Commit/BuiltAt`
- 构建示例：
  - `docker build -t waveyo/ip-api:latest .`
  - 或显式传参：`docker build --build-arg GIT_SHA=$(git rev-parse --short HEAD) --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) -t waveyo/ip-api:latest .`

**KV 覆盖（快速修正）**
- 表结构：`_ip_overrides_kv(assoc_key TEXT, ip_int BIGINT, country, region, province, city, isp, updated_at)`，主键 `(assoc_key, ip_int)`。位置：`internal/migrate/schema.go:41-59`
- 查询优先级：DB 回退首先查 KV；文件缓存在启动时合并 KV 生成 `exact.db`

**KV CLI 使用**
- 构建：`go build ./cmd/override-kv`
- 连接方式：
  - 手动输入：运行后按提示输入 `PG_HOST/PG_PORT/PG_USER/PG_PASSWORD/PG_DB/PG_SSLMODE`
  - 指定 env 文件：`.\n+    override-kv.exe --env .env` 或传 `.env` 文件路径
- 命令：
  - `add <key> <ip> <country> <region> [province] [city] [isp]`
  - `set <key> <ip> <country> <region> [province] [city] [isp]`
  - `del <key> <ip>`
  - `get <key> <ip>`
  - `list <key> [limit]`
  - `keys <ip>`（通过 IP 返回所有关联的 `assoc_key`）
- 示例（修正 1.1.1.1 为 CLOUDFLARE）：
  - `set global 1.1.1.1 CLOUDFLARE.COM CLOUDFLARE.COM`
  - 验证：`Invoke-WebRequest -Uri "http://localhost:8080/api/ip?ip=1.1.1.1" | % { $_.Content }`
  - 若需文件缓存优先命中，请重启后端以重建 `exact.db`

**CDN 缓存绕过建议**
- 源站核对：`Invoke-WebRequest -Uri "http://localhost:8080/api/ip?ip=1.1.1.1"`
- 走 CDN 避免命中：`Invoke-WebRequest -Uri ("https://ip.waveyo.cn/api/ip?ip=1.1.1.1&cb=" + [guid]::NewGuid()) -Headers @{ "Cache-Control"="no-cache, no-store"; "Pragma"="no-cache" }`

**常见问题**
- 版本显示 `dev/空`：确认 Docker 构建时 `.git` 在上下文内，或显式传入 `--build-arg`；接口回退逻辑在 `internal/api/ip-api.go:199-204`
- 仍显示“保留地址”：检查文件缓存是否仍命中旧值；先用源站接口验证，再通过 KV 修正并重启后端使文件库生效。
