# IP 归属地查询（后端 API + 本地分片文件缓存，数据库回退）

提供基于本地分片文件缓存的高并发 IPv4 查询服务（首段分片 + 二分查找）。当文件缓存未命中时回退 PostgreSQL 范围表查询（仅 IPv4）。前端通过后端 API 获取结果。

**功能概览**
- 后端接口：`GET /api/ip?ip=...`，自动识别客户端 IP（多层代理头部处理），返回 `country | region | province | city | isp`。
- 缓存：命中结果写入 Redis（可选），提升热点查询命中率。
- 指标：`GET /api/stats` 返回总计与当日服务量；前端展示服务量卡片。
- 数据维护：支持从本地 IPIP 数据源并行导入到数据库，并构建分片文件缓存；可选离线拉取上游文本源（ip2region）导入数据库。

**目录结构**
- 后端入口：`cmd/main.go`
- API 路由：`internal/api/ip-api.go`
- IPIP 读取与枚举：`internal/ipip/ipip.go`
- IPIP 多线程导入 PostgreSQL：`internal/ipip/importer.go`（`ImportIPv4LeavesToDBConcurrent`）
- 本地文件分片缓存构建与读取：`internal/localdb/filecache.go`
- 数据库访问层：`internal/store/store.go`
- 数据导入与调度：`internal/ingest/ingest.go`、`internal/ingest/scheduler.go`
- 前端应用：`ui/`

**运行与部署**
- 环境变量（节选）：
  - `API_BASE`：API 前缀，默认 `/api`
  - `PG_HOST/PG_PORT/PG_USER/PG_PASSWORD/PG_DB/PG_SSLMODE`：PostgreSQL 连接参数
  - `REDIS_HOST/REDIS_PORT/REDIS_PASS/REDIS_DB`：Redis 参数（可选）
- `IPIP_PATH`：本地 IPIP 数据源路径（默认 `data/ipip/ipipfree.ipdb`）
- `IPIP_LANG`：IPIP 解析语言（默认 `zh-CN`）
- `IPIP_WORKERS`：导入并行度（默认 `8`）
- `DEDUP_TTL_SECONDS`：请求去重布隆过滤器 TTL（默认 `600`）
- `ADMIN_TOKEN`：后台重载缓存令牌（`X-Admin-Token`）
  - 启动：
  - 后端：`go build ./... && ./ip-api` 或 `ADDR=:8080` 运行在指定端口
  - 前端：`ui` 目录下 `pnpm install && pnpm run dev`（开发）或构建产物部署在静态目录

**数据维护机制**
- 数据库导入：
  - 源：`https://raw.githubusercontent.com/lionsoul2014/ip2region/master/data/ipv4_source.txt`
  - 使用方式：作为可选离线任务，按需调用 `internal/ingest` 的初始化与定时任务；默认运行主流程不启用。
- IPIP 导入：
  - 源：本地 `data/ipip/ipipfree.ipdb`
  - 初始化：服务启动时如范围表为空，优先并行枚举 IPv4 叶子并写库（`ImportIPv4LeavesToDBConcurrent`）。

**查询路径与回退策略**
- 优先使用文件分片缓存（`internal/localdb/filecache.go`）：按首段加载分片并二分查询。
- 回退至数据库（仅 IPv4）：当本地缓存未命中时查询范围表（`internal/store/store.go:50-62`）。
- 前端透传接口基础路径与数据源声明：`/config.js` 提供 `window.__API_BASE__`、`window.__DATA_SOURCE__='IPIP 数据库'`、`window.__DATA_SOURCE_URL__='https://www.ipip.net'`（`cmd/main.go:141-150`）。

**致谢**
- 数据源与工具：IPIP 数据库、ip2region 数据集
- 前端框架：Vite + Vue 3
