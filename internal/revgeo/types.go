package revgeo

import "time"

// 文档注释：行政区与空间索引的最小数据结构
// 背景：统一承载国/省/市等层级的元数据与几何；保持轻量以便常驻内存与快速判定。
// 约束：几何仅支持 GeoJSON 的 Polygon/MultiPolygon；多面与洞以环列表表达，第一环为外环，其余为洞。
type AdminUnit struct {
    Country  string
    Region   string
    Province string
    City     string
    Polys    []Polygon
}

// Polygon：按 GeoJSON 约定的环集合，第一环是外环，其后为洞
type Polygon struct {
    Rings [][]Point
    BBox  [4]float64 // minLon, minLat, maxLon, maxLat
}

// 点坐标（WGS84）
type Point struct { Lat float64; Lon float64 }

// 质心表项（用于 KD-Tree 最近邻兜底）
type Centroid struct {
    Lat float64
    Lon float64
    Country  string
    Region   string
    Province string
    City     string
}

// 加载结果快照：只读引用，供查询期共享
type Snapshot struct {
    Units     []AdminUnit
    Centroids []Centroid
    BuiltAt   time.Time
}

