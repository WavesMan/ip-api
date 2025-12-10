package revgeo

import "math"

// 文档注释：KD-Tree 最近邻（二维经纬）
// 背景：在 PIP 未命中或边界附近不稳定时提供城市级兜底；限制最大半径避免海上或偏远地点误归属。
// 约束：使用简化二叉树构建，按经度优先/纬度交替分割；仅支持最近一个点查询。
type kdNode struct {
    c   Centroid
    ax  int // 0:lon,1:lat
    l   *kdNode
    r   *kdNode
}

func buildKD(cs []Centroid, depth int) *kdNode {
    if len(cs) == 0 { return nil }
    ax := depth % 2
    // 选择中位数分割，避免外部排序带来的额外依赖
    mid := len(cs) / 2
    selectNth(cs, mid, ax)
    node := &kdNode{c: cs[mid], ax: ax}
    node.l = buildKD(cs[:mid], depth+1)
    node.r = buildKD(cs[mid+1:], depth+1)
    return node
}

// 原地 nth 元素选择（轴为经度/纬度）
func selectNth(a []Centroid, n int, ax int) {
    lo, hi := 0, len(a)-1
    for lo < hi {
        p := partition(a, lo, hi, (lo+hi)/2, ax)
        if p == n { return }
        if n < p { hi = p - 1 } else { lo = p + 1 }
    }
}

func partition(a []Centroid, lo, hi, pivot, ax int) int {
    pv := a[pivot]
    a[pivot], a[hi] = a[hi], a[pivot]
    i := lo
    for j := lo; j < hi; j++ {
        if lessCent(a[j], pv, ax) { a[i], a[j] = a[j], a[i]; i++ }
    }
    a[i], a[hi] = a[hi], a[i]
    return i
}

func lessCent(x, y Centroid, ax int) bool {
    if ax == 0 { return x.Lon < y.Lon }
    return x.Lat < y.Lat
}

// 最近邻查询，返回距离（千米）与质心
func nearest(node *kdNode, pt Point) (Centroid, float64) {
    best := Centroid{}
    bestD := math.MaxFloat64
    var dfs func(n *kdNode)
    dfs = func(n *kdNode) {
        if n == nil { return }
        d := haversine(pt.Lat, pt.Lon, n.c.Lat, n.c.Lon)
        if d < bestD { bestD = d; best = n.c }
        var key, q float64
        if n.ax == 0 { key = pt.Lon; q = n.c.Lon } else { key = pt.Lat; q = n.c.Lat }
        first, second := n.l, n.r
        if key > q { first, second = n.r, n.l }
        dfs(first)
        // 仅当分割平面到查询点的距离小于当前最优距离时才遍历另一侧
        if math.Abs(key-q) < bestD/111.0 { dfs(second) }
    }
    dfs(node)
    return best, bestD
}

// 球面距离（Haversine），返回千米
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
    const R = 6371.0
    dLat := (lat2 - lat1) * math.Pi / 180
    dLon := (lon2 - lon1) * math.Pi / 180
    a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLon/2)*math.Sin(dLon/2)
    c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
    return R * c
}

