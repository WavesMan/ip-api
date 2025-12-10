package api

// 文档注释：查询返回结构（对外）
// 背景：统一对外序列化模型，仅包含必要字段，避免泄露内部差异；便于缓存与统计一致化处理。
// 约束：字段稳定；新增字段需评估兼容性与前端依赖。
type queryResult struct {
    IP       string `json:"ip"`
    Country  string `json:"country"`
    Region   string `json:"region"`
    Province string `json:"province"`
    City     string `json:"city"`
    ISP      string `json:"isp"`
}

