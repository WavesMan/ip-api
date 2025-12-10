package revgeo

// 文档注释：点入多边形判定（Even-Odd）
// 背景：对候选集合执行精确命中判定；支持洞与多面结构；用于返回最细层级的行政区。
// 约束：输入为经纬度坐标（WGS84）；射线算法在边界临界值时易受数值误差影响，需结合边界距离做稳定性处理。
func pointInPoly(pt Point, poly Polygon) bool {
    // 外环命中且不在洞内视为命中
    if len(poly.Rings) == 0 { return false }
    if !pointInRing(pt, poly.Rings[0]) { return false }
    for i := 1; i < len(poly.Rings); i++ {
        if pointInRing(pt, poly.Rings[i]) { return false }
    }
    return true
}

// 射线法判定点是否在环内
func pointInRing(pt Point, ring []Point) bool {
    n := len(ring)
    if n < 3 { return false }
    inside := false
    x := pt.Lon
    y := pt.Lat
    for i, j := 0, n-1; i < n; j, i = i, i+1 {
        xi := ring[i].Lon; yi := ring[i].Lat
        xj := ring[j].Lon; yj := ring[j].Lat
        intersect := ((yi > y) != (yj > y)) && (x < (xj-xi)*(y-yi)/(yj-yi+1e-12)+xi)
        if intersect { inside = !inside }
    }
    return inside
}

// 快速包围盒过滤
func inBBox(pt Point, b [4]float64) bool {
    return pt.Lon >= b[0] && pt.Lon <= b[2] && pt.Lat >= b[1] && pt.Lat <= b[3]
}

