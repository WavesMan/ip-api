package revgeo

import (
    "math"
    "os"
    "strconv"
)

// 文档注释：查询编排器（R-Tree候选 → PIP命中 → 网格/最近邻兜底）
// 背景：统一调度多索引以实现城市级反地理；结合近似标记与置信度输出，便于与现有融合体系衔接。
// 约束：当前 R-Tree 以包围盒线性过滤近似；后续可替换为分层 R-Tree；网格索引仅用于邻接探测与缓存键。
type Orchestrator struct {
    snap *Snapshot
    kd   *kdNode
    cache *LRU
    maxRadiusKm float64
}

// 构造编排器，读取环境变量作为参数
func NewOrchestrator(snap *Snapshot) *Orchestrator {
    ttlSec := 3600
    if s := os.Getenv("REVERSE_GEO_CACHE_TTL_S"); s != "" { if n, e := strconv.Atoi(s); e == nil && n > 0 { ttlSec = n } }
    c := NewLRU(4096, ttlSec)
    r := 50.0
    if s := os.Getenv("REVERSE_GEO_KDTREE_RADIUS_KM"); s != "" { if f, e := strconv.ParseFloat(s, 64); e == nil && f > 0 { r = f } }
    var kd *kdNode
    if snap != nil && len(snap.Centroids) > 0 { kd = buildKD(append([]Centroid{}, snap.Centroids...), 0) }
    return &Orchestrator{snap: snap, kd: kd, cache: c, maxRadiusKm: r}
}

// 文档注释：反地理查询（返回行政区与置信度、是否近似）
// 背景：输入坐标统一视为 WGS84；国内来源可选 GCJ-02/BD-09 转换。
// 返回：命中行政区与置信度，approx 表示为非 PIP 精确命中（网格/最近邻）。
func (o *Orchestrator) Query(lat, lon float64, coordSys string) (AdminUnit, float64, bool) {
    if coordSys != "" && stringsEqualCI(coordSys, "GCJ-02") { lat, lon = gcj02ToWGS84(lat, lon) }
    if coordSys != "" && stringsEqualCI(coordSys, "BD-09") { lat, lon = bd09ToWGS84(lat, lon) }
    key := encodeGeohash(lat, lon, 6)
    if v, ok := o.cache.Get(key); ok {
        // 近似标记不在缓存中携带，由调用方根据来源判断；此处返回命中行政区即可
        return v, 0.7, true
    }
    pt := Point{Lat: lat, Lon: lon}
    // 候选过滤（包围盒）
    var candIdx []int
    for i := range o.snap.Units {
        u := &o.snap.Units[i]
        for _, p := range u.Polys {
            if inBBox(pt, p.BBox) { candIdx = append(candIdx, i); break }
        }
    }
    // PIP 精确判定
    for _, idx := range candIdx {
        u := o.snap.Units[idx]
        for _, p := range u.Polys {
            if pointInPoly(pt, p) {
                o.cache.Set(key, u)
                // 城市级命中
                if u.City != "" { return u, 0.9, false }
                if u.Province != "" { return u, 0.8, false }
                if u.Region != "" || u.Country != "" { return u, 0.7, false }
            }
        }
    }
    // 最近邻兜底（KD-Tree）
    if o.kd != nil {
        c, d := nearest(o.kd, pt)
        if d <= o.maxRadiusKm {
            u := AdminUnit{Country: c.Country, Region: c.Region, Province: c.Province, City: c.City}
            o.cache.Set(key, u)
            conf := 0.6
            if d > 30 { conf = 0.5 }
            return u, conf, true
        }
    }
    // 海上或远离城市：仅返回国家/省级（若可推断）
    // 当前快照未必包含海岸线，返回空以供融合兜底
    return AdminUnit{}, 0.0, true
}

// 文档注释：坐标系转换（GCJ-02/BD-09 → WGS84）
// 背景：国内互联网地图坐标需转换以贴合全球边界数据；避免过度转换导致偏移。
// 约束：简化实现，误差在数十米级；仅当来源明确为 GCJ-02/BD-09 时启用。
func gcj02ToWGS84(lat, lon float64) (float64, float64) {
    glat, glon := transformGCJ(lat, lon)
    return lat*2 - glat, lon*2 - glon
}

func bd09ToWGS84(lat, lon float64) (float64, float64) {
    // BD-09 -> GCJ-02 -> WGS84
    x := lon - 0.0065
    y := lat - 0.006
    z := math.Sqrt(x*x+y*y) - 0.00002*math.Sin(y*math.Pi)
    theta := math.Atan2(y, x) - 0.000003*math.Cos(x*math.Pi)
    gcjLon := z * math.Cos(theta)
    gcjLat := z * math.Sin(theta)
    return gcj02ToWGS84(gcjLat, gcjLon)
}

func transformGCJ(lat, lon float64) (float64, float64) {
    if outOfChina(lat, lon) { return lat, lon }
    dLat := transformLat(lon-105.0, lat-35.0)
    dLon := transformLon(lon-105.0, lat-35.0)
    radLat := lat / 180.0 * math.Pi
    magic := math.Sin(radLat)
    magic = 1 - 0.00669342162296594323*magic*magic
    sqrtMagic := math.Sqrt(magic)
    dLat = (dLat * 180.0) / ((6378245.0 * (1 - 0.00669342162296594323)) / (magic * sqrtMagic) * math.Pi)
    dLon = (dLon * 180.0) / (6378245.0 / sqrtMagic * math.Cos(radLat) * math.Pi)
    mgLat := lat + dLat
    mgLon := lon + dLon
    return mgLat, mgLon
}

func outOfChina(lat, lon float64) bool { return lon < 72.004 || lon > 137.8347 || lat < 0.8293 || lat > 55.8271 }

func transformLat(x, y float64) float64 {
    ret := -100.0 + 2.0*x + 3.0*y + 0.2*y*y + 0.1*x*y + 0.2*math.Sqrt(math.Abs(x))
    ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
    ret += (20.0*math.Sin(y*math.Pi) + 40.0*math.Sin(y/3.0*math.Pi)) * 2.0 / 3.0
    ret += (160.0*math.Sin(y/12.0*math.Pi) + 320*math.Sin(y*math.Pi/30.0)) * 2.0 / 3.0
    return ret
}

func transformLon(x, y float64) float64 {
    ret := 300.0 + x + 2.0*y + 0.1*x*x + 0.1*x*y + 0.1*math.Sqrt(math.Abs(x))
    ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
    ret += (20.0*math.Sin(x*math.Pi) + 40.0*math.Sin(x/3.0*math.Pi)) * 2.0 / 3.0
    ret += (150.0*math.Sin(x/12.0*math.Pi) + 300.0*math.Sin(x/30.0*math.Pi)) * 2.0 / 3.0
    return ret
}

func stringsEqualCI(a, b string) bool {
    if len(a) != len(b) { return false }
    for i := 0; i < len(a); i++ {
        ca := a[i]; cb := b[i]
        if ca >= 'A' && ca <= 'Z' { ca = ca + 32 }
        if cb >= 'A' && cb <= 'Z' { cb = cb + 32 }
        if ca != cb { return false }
    }
    return true
}

