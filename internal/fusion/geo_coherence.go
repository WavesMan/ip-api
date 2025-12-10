package fusion

import "strings"

// 文档注释：判定是否中国语义（省市/直辖市/特别行政区）
// 背景：用于融合时的一致性校验与惩罚/兜底；当区域/城市明显属于中国时，国家应为中国，否则判定为冲突。
// 约束：词典为常见省级/直辖市/特区集合，覆盖主要场景；不追求穷尽。
func isChinaLike(loc Location) bool {
    if strings.EqualFold(loc.Country, "中国") { return true }
    provinces := []string{
        "广东","浙江","上海","北京","江苏","山东","河南","河北","湖南","湖北",
        "福建","安徽","江西","辽宁","吉林","黑龙江","云南","贵州","四川","重庆",
        "天津","山西","内蒙古","广西","海南","宁夏","新疆","西藏","青海","甘肃",
        "香港","澳门","台湾",
    }
    r := loc.Region
    c := loc.City
    for _, p := range provinces {
        if r == p || c == p { return true }
    }
    // 简易中文字符判断：出现常见汉字则认为更可能为中国语义（弱信号）
    zhHints := []string{"省","市","自治区","特别行政区"}
    for _, h := range zhHints {
        if strings.Contains(r, h) || strings.Contains(c, h) { return true }
    }
    return false
}

// 文档注释：一致性系数（国家与区域/城市不一致时惩罚）
// 背景：当区域/城市显然属于中国而国家非中国，降低该来源的综合分数，避免被多数投票选入。
// 返回：一致时为 1.0；不一致时为 0.7（惩罚可按需调整）。
func CoherenceCoeff(loc Location) float64 {
    if isChinaLike(loc) && !strings.EqualFold(loc.Country, "中国") {
        return 0.7
    }
    return 1.0
}

