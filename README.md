# IP 归属地查询（静态资产 + Worker，零外部依赖）

提供离线 IP 归属地查询能力：前端直接从静态二进制数据解析并查询；可选通过 Worker 暴露 `GET /api/ip?ip=...` 供外部系统调用。适配 EdgeOne Pages 等纯静态托管平台。

## 功能概览
- 前端查询：页面输入 IPv4，前端加载 `/db/*` 静态数据并本地查询
- 外部接口：`GET /api/ip?ip=...`（由 `workers/ip-api.js` 提供）
- 返回字段：`country | province | city`

## 架构设计
- 数据源：`ip2region`（IPv4 数据）
- 构建产物：
  - 字典 `dict.bin`：字符串池与三元组索引
  - 分片 `chunks/a{0..255}.bin`：按首段分片的范围记录，变长编码
- 运行时：
  - 前端与 Worker 共享解析逻辑（二进制读取 + 二分查找）
  - Worker 从同域静态资源 `/db/*` 读取并返回 JSON

## 目录结构
```
public/db/
  dict.bin
  chunks/a{0..255}.bin
scripts/
  build-db.mjs
src/
  App.vue
  main.js
workers/
  ip-api.js
```

## 快速开始（本地）
- 环境：`Node.js >= 18`、`pnpm >= 8`
- 安装依赖：
```
pnpm install
```
- 构建静态数据库：
```
pnpm run build:db
```
- 启动前端：
```
pnpm run dev
```
访问 `http://localhost:5173/`，在页面输入 IP 即可查询。

## 部署建议
- 纯静态：部署 `public/db` 与前端产物，用户在页面查询（无 `/api/ip`）
- 保留接口：部署 Worker 并路由到 `/api/ip`，Worker 内部读取同域 `/db/*` 并返回 JSON
  - Cloudflare/Tencent EdgeOne 等平台均可使用本 Worker 逻辑

## API 说明（可选）
- 路径：`GET /api/ip?ip=1.2.3.4`
- 响应：
```
{ "ip": "1.2.6.1", "country": "中国", "province": "福建省", "city": "福州市" }
```
- 说明：当前接口需显式传入 `ip` 参数（未实现从请求头自动识别）

## 数据与体积
- 字典体积：几十 KB（随数据源变化）
- 分片总计：约数 MB，按需加载首段文件
- 查询耗时：热态二分查找近似 O(log n)，前端与 Worker 共享逻辑

## 相关文件位置
- 前端解析与查询：`src/App.vue:14-128`
- Worker 接口实现：`workers/ip-api.js:1`
- 构建脚本：`scripts/build-db.mjs`

## 致谢
- 数据源与工具：`lionsoul2014/ip2region`
- 前端框架：Vite + Vue 3
