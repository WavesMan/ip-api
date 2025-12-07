# IP 归属地查询服务（EdgeOne Pages + 无外部依赖）

离线、自动更新、适配边缘函数限制的 IP 归属地查询服务。提供 `GET /api/ip?ip=...` 接口返回国家/省份/城市，部署在 Tencent EdgeOne Pages，运行时零外部依赖，Bundle ≤ 5MB，冷启动 ≤ 300ms。

## 功能概览
- 接口：`/api/ip?ip=...`（IPv4），不传 `ip` 则从请求头识别访问者外网 IP
- 返回字段：`country | province | city`
- 前端页面：展示访问者归属地；支持输入 IP 搜索；展示累计/今日调用次数
- 自动更新：每周一定时拉取数据并构建；推送 `pages` 分支自动部署
- 计数统计：使用 EdgeOne Pages KV（变量名 `view_stats`）累计总次数与按日次数

## 架构设计
- 数据源：`ip2region` 的 `data/ipv4_source.txt`
- 预处理与压缩：构建脚本将原始数据转为二进制字典（`strings + triples`）与按首段分片（256 chunk）的记录文件，记录采用变长编码（`startDelta + length + tripleIndex`）
- 运行时加载：边缘函数首次懒解析字典，按 IP 首段动态加载分片并二分查找；实例复用下热请求 < 1ms
- 自动化部署：GitHub Actions 构建数据与前端，发布到 `pages` 分支，Pages 平台拉取部署

## 目录结构
```
edge-functions/
  dict.js            # 字典二进制（自动生成）
  chunks/a{0..255}.js# 分片二进制（自动生成）
  ip-lookup.js       # /api/ip 边缘函数（动态加载 + 查询 + 计数）
  stats.js           # /api/stats 边缘函数（聚合读取计数）
scripts/
  build-db.mjs       # 解析/压缩/生成 dict 和 chunks
.github/workflows/
  update-db.yml      # 周一定时 + main 分支 push 构建并发布到 pages 分支
edgeone.config.js    # 路由映射
src/                 # Vite + Vue 前端（JS）
```

## 快速开始（本地开发）
环境要求：`Node.js >= 24 LTS`、`pnpm >= 10`

1. 安装依赖
```
pnpm install
```
2. 构建数据（首次需要）
```
pnpm run build:db
```
3. 启动前端（内置中间件代理到本地边缘函数模块）
```
pnpm run dev
```
访问 `http://localhost:5173/`，页面将显示访问者归属地（开发态无真实外网 IP，显示为空），支持输入 IP 查询。

接口联调：
```
GET http://localhost:5173/api/ip?ip=8.8.8.8
GET http://localhost:5173/api/stats
```

## 部署到 EdgeOne Pages
1. 在仓库 Settings → Actions 将 `Workflow permissions` 设为 `Read and write permissions`
2. 在 EdgeOne Pages 控制台开通 KV，并创建命名空间后绑定到项目，**变量名**设为 `view_stats`
3. 推送到 `main` 分支或等待周一定时任务，GitHub Actions 会：
   - `pnpm run build:db` 生成 `dict.js` 与 `chunks/`
   - `pnpm run build` 生成前端 `dist/`
   - 组装部署内容到 `pages` 分支并强制推送
4. 在 Pages 项目中配置从 `pages` 分支部署（静态 + 边缘函数）

路由配置：`edgeone.config.js`
```
{ path: "/api/ip", function: "./edge-functions/ip-lookup.js" }
{ path: "/api/stats", function: "./edge-functions/stats.js" }
```

## API 说明
### 查询接口 `/api/ip`
- 请求：`GET /api/ip?ip=1.2.3.4`
- 不传 `ip`：从请求头获取访问者外网 IP（支持 `x-forwarded-for`、`forwarded`、`cf-connecting-ip`、`x-real-ip`、`x-client-ip`、`x-edge-client-ip/x-edgeone-ip`）
- 响应：
```
{ "ip": "1.2.6.1", "country": "中国", "province": "福建省", "city": "福州市" }
```

### 统计接口 `/api/stats`
- 请求：`GET /api/stats`
- 响应：
```
{ "total": 12345, "today": 678 }
```

## KV 计数器设计（变量名：`view_stats`）
- 键模型（分片 16 片，降低写热点）：
  - 总计数：`req:total:{s}`，`{s} ∈ [0..15]`
  - 当日计数：`req:{YYYYMMDD}:{s}`（UTC 日期）
- 写入：每次成功查询后并行递增总计数与当日计数
- 读取：聚合 16 分片求和并返回；可选在函数内做 30–60s 内存缓存
- 保留策略：当日计数建议 TTL 400 天；总计数永久

## 性能与体积
- 首包：字典 ~46KB + 加载器代码
- 分片：合计约 3.8MB，按需动态加载（实例复用下热查询 < 1ms）
- 冷启动：首次解析字典 + 加载一个分片，通常 ≤ 250ms

## 前端（UI/UX）
- 风格参考 `Koishi-Registry`：卡片化信息区、极简浅色主题、分区清晰
- 页面功能：访问者归属地展示、IP 搜索、累计/今日次数统计
- 入口：`src/App.vue`

## 注意事项与限制
- 当前实现支持 IPv4；IPv6 可按同构建与查询流程扩展
- 数据来源用于城市级归属地查询，精度与时效取决于来源数据；本项目不存储用户明细
- EdgeOne Pages KV 遵循最终一致性，统计在 60s 内达到一致

## 常见问题
- 本地开发为何访问者 IP 为空：开发服务器不会提供真实外网 IP 头部；部署到边缘节点后将由网关注入真实头部
- 304 响应：开发态已设置 `cache-control: no-store` 和中间件直返 JSON，避免缓存导致 304

## 许可与致谢
- 数据源与工具：`lionsoul2014/ip2region`
- 前端框架：Vite + Vue 3
