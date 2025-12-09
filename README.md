# IP 归属地查询服务（插件化融合）

高并发 IPv4 查询，优先本地文件缓存，未命中回退 PostgreSQL。支持 KV 覆盖快速修正（如修正 1.1.1.1 显示“保留地址”）。

**接口与能力**
- `GET /api/ip?ip=...` 返回 `country/region/province/city/isp`（自动识别客户端 IP）。实现位置：`internal/api/ip-api.go`
- `GET /api/stats` 返回总计与当日服务量。实现位置：`internal/api/ip-api.go`
- `GET /api/version` 返回 `commit` 与 `builtAt`。实现位置：`internal/api/ip-api.go:199-204`
- Redis 热点缓存（可选），TTL 可配置：`CACHE_TTL_SECONDS`。命中逻辑：`internal/api/ip-api.go:244-276,309-317`
- 去重布隆过滤器窗口：`DEDUP_TTL_SECONDS`。实现位置：`internal/api/ip-api.go:214-234,154-180`

**查询路径优先级**
- 精确文件库命中（ExactDB）→ 前缀树缓存（IPIP）→ 数据库回退（范围或特例）→ 插件并发融合落库；组合器：`internal/localdb/multicache.go`，插件管理：`internal/plugins/`
- DB 回退优先检查 KV 覆盖：`internal/store/store.go:62-70`；插件融合结果在满足阈值（≥80）时写 `_ip_exact` 并异步重建 `ExactDB`
- 启动时自动构建精确文件库：如 `_ip_overrides` 或 `_ip_overrides_kv` 有数据则生成 `exact.db` 并加载。位置：`cmd/main.go`
- 精确文件库构建时合并 KV：`internal/localdb/exactdb.go`

**目录结构**
- 后端入口：`cmd/main.go`
- API 路由：`internal/api/ip-api.go`
- 数据库层：`internal/store/store.go`
- 本地缓存：`internal/localdb/`
- 插件管理与适配：`internal/plugins/`（`manager.go`、`http_plugin.go`、`amap.go`、`ip2region.go`）
- 版本信息：`internal/version/version.go`
- 前端应用：`ui/`
- KV 覆盖 CLI：`cmd/override-kv/main.go`

**环境变量（核心）**
- `ADDR` 服务地址，默认 `:8080`
- `API_BASE` API 前缀，默认 `/api`
- `UI_DIST` 前端静态目录，默认 `ui/dist`
- `PG_*` 数据库连接（`PG_HOST/PG_PORT/PG_USER/PG_PASSWORD/PG_DB/PG_SSLMODE`）
- `REDIS_*` Redis 参数（可选）
- `CACHE_TTL_SECONDS`、`DEDUP_TTL_SECONDS` 缓存与去重 TTL（秒）
- `IPIP_PATH` 本地 IPIP 数据源路径，默认 `data/ipip/ipipfree.ipdb`
- `IP2REGION_V4_PATH` IP2Region v4 数据文件路径（可选）
- `AMAP_SERVER_KEY` 高德服务端密钥（可选，启用在线 AMap 插件）
- 权重微调：`FUSION_WEIGHT_KV`、`FUSION_WEIGHT_IPIP`、`FUSION_WEIGHT_IP2R`、`FUSION_WEIGHT_AMAP`（范围建议 1–10）
- 外部插件（HTTP）：`EXT_PLUGIN_ENDPOINT/NAME/ASSOC/WEIGHT`
 - 不完整触发融合：`ENABLE_FUSION_ON_PARTIAL_CACHE`、`ENABLE_FUSION_ON_PARTIAL_DB`
 - 最小分阈值：`FUSION_MIN_SCORE_ON_CACHE`（默认 20）
 - 额外 env 加载路径：后端会尝试加载 `data/env/.env`

**Docker 构建**
- 强制读取 `.git` 注入版本：`Dockerfile:29-36` 显式 `COPY .git .git`，构建阶段 `git rev-parse`/`git log` 自动注入 `Commit/BuiltAt`
- 构建示例：
  - `docker build -t waveyo/ip-api:latest .`
  - 或显式传参：`docker build --build-arg GIT_SHA=$(git rev-parse --short HEAD) --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) -t waveyo/ip-api:latest .`

**KV 覆盖（快速修正）**
- 表结构：`_ip_overrides_kv(assoc_key TEXT, ip_int BIGINT, country, region, province, city, isp, updated_at)`，主键 `(assoc_key, ip_int)`。位置：`internal/migrate/schema.go`
- 查询优先级：DB 回退首先查 KV；文件缓存在启动时合并 KV 生成 `exact.db`；插件融合结果满足覆盖与阈值策略时写库

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

**插件架构说明**
- 插件契约：`Query(ctx, ip)->Location, confidence`、`GetWeight(ip)->float64`、`Heartbeat()->error`；`AssocKey()` 用于落库时按来源分域（权限域/覆盖策略）。
- 管理层：`PluginManager` 负责注册、心跳/健康筛选；提供“健康插件集合”给融合层。
- 融合层：评分模型 `score=100×(weight/10)×qualityCoeff×confidence`；Top3 字段级多数投票，无多数取最高分。
- 写库层：KV 覆盖优先（`new_score>old+20`），Exact 满足阈值（默认≥80）落 `_ip_exact`；随后重建 `ExactDB` 并原子热切换。
- 缓存层：链式缓存组合 `ExactDB→IPIP→IP2Region`，通过 `DynamicCache.Set()` 热切换。
 - 前端等待提示：当查询进行中，界面显示“数据库数据不完整，正在分析…”。
- `TLS_ENABLE` 是否启用 TLS（默认 `true`，仅 HTTPS 服务，不切换至 443）
- `TLS_CERT_PATH/TLS_KEY_PATH` 自签证书路径（默认 `data/certs/server.crt`、`data/certs/server.key`；启动时自动生成）
- `TLS_REDIRECT_ENABLE` 是否开启 HTTP→HTTPS 重定向（默认 `true`）
- `TLS_REDIRECT_ADDR` 重定向监听地址（默认 `:80`），将 80 上的 HTTP 访问重定向到 `https://<host>:<ADDR端口>`
- 额外 env 加载路径：后端会尝试加载 `data/env/.env`
- 安全说明：默认启用 TLS，自签证书在首次启动自动生成；请使用 `https://<host>:<port>` 访问，HTTP 请求将被拒绝（未启动明文服务）。
