package revgeo

import (
    "encoding/json"
    "io"
    "os"
    "path/filepath"
    "strings"
)

// 文档注释：从数据目录加载边界与质心快照
// 背景：支持从 Natural Earth/geoBoundaries/自建城市质心的 GeoJSON/JSON 文件读取；构建轻量快照用于查询。
// 约束：约定文件名：boundaries.json 或 *.geojson（行政边界），city_centroids.json（质心）；缺失文件时仅提供最近邻兜底。
func LoadSnapshot(dir string) (*Snapshot, error) {
    var units []AdminUnit
    var cents []Centroid
    // 质心
    centPath := filepath.Join(dir, "city_centroids.json")
    if b, err := os.ReadFile(centPath); err == nil {
        _ = json.Unmarshal(b, &cents)
    }
    // 边界：优先 boundaries.json，其次扫描 .geojson
    b0 := filepath.Join(dir, "boundaries.json")
    if b, err := os.ReadFile(b0); err == nil {
        var raw []map[string]any
        if e := json.Unmarshal(b, &raw); e == nil {
            for _, r := range raw {
                units = append(units, parseUnit(r))
            }
        }
    } else {
        entries, _ := os.ReadDir(dir)
        for _, ent := range entries {
            name := ent.Name()
            if strings.HasSuffix(strings.ToLower(name), ".geojson") {
                fp := filepath.Join(dir, name)
                if f, e := os.Open(fp); e == nil {
                    bs, _ := io.ReadAll(f)
                    _ = f.Close()
                    var gj map[string]any
                    if e2 := json.Unmarshal(bs, &gj); e2 == nil {
                        addUnitsFromGeoJSON(&units, gj)
                    }
                }
            }
        }
    }
    snap := &Snapshot{Units: units, Centroids: cents}
    return snap, nil
}

func parseUnit(r map[string]any) AdminUnit {
    var u AdminUnit
    if v, ok := r["country"].(string); ok { u.Country = v }
    if v, ok := r["region"].(string); ok { u.Region = v }
    if v, ok := r["province"].(string); ok { u.Province = v }
    if v, ok := r["city"].(string); ok { u.City = v }
    if g, ok := r["geometry"].(map[string]any); ok {
        addPolysFromGeometry(&u, g)
    }
    return u
}

// 解析 GeoJSON FeatureCollection/Feature
func addUnitsFromGeoJSON(units *[]AdminUnit, gj map[string]any) {
    t := strings.ToLower(getStr(gj, "type"))
    if t == "featurecollection" {
        if arr, ok := gj["features"].([]any); ok {
            for _, it := range arr {
                if f, ok := it.(map[string]any); ok {
                    var u AdminUnit
                    if p, ok := f["properties"].(map[string]any); ok {
                        u.Country = getStr(p, "country")
                        u.Region = getStr(p, "region")
                        u.Province = getStr(p, "province")
                        u.City = getStr(p, "city")
                    }
                    if g, ok := f["geometry"].(map[string]any); ok { addPolysFromGeometry(&u, g) }
                    *units = append(*units, u)
                }
            }
        }
        return
    }
    if t == "feature" {
        var u AdminUnit
        if p, ok := gj["properties"].(map[string]any); ok {
            u.Country = getStr(p, "country")
            u.Region = getStr(p, "region")
            u.Province = getStr(p, "province")
            u.City = getStr(p, "city")
        }
        if g, ok := gj["geometry"].(map[string]any); ok { addPolysFromGeometry(&u, g) }
        *units = append(*units, u)
    }
}

func addPolysFromGeometry(u *AdminUnit, g map[string]any) {
    gt := strings.ToLower(getStr(g, "type"))
    if gt == "polygon" {
        var poly Polygon
        if coords, ok := g["coordinates"].([]any); ok {
            for _, ring := range coords {
                if arr, ok := ring.([]any); ok {
                    var rr []Point
                    for _, p := range arr {
                        if vv, ok := p.([]any); ok && len(vv) >= 2 {
                            lon := toFloat(vv[0]); lat := toFloat(vv[1])
                            rr = append(rr, Point{Lat: lat, Lon: lon})
                        }
                    }
                    poly.Rings = append(poly.Rings, rr)
                }
            }
        }
        poly.BBox = computeBBox(poly)
        u.Polys = append(u.Polys, poly)
        return
    }
    if gt == "multipolygon" {
        if coords, ok := g["coordinates"].([]any); ok {
            for _, part := range coords {
                var poly Polygon
                if rings, ok := part.([]any); ok {
                    for _, ring := range rings {
                        if arr, ok := ring.([]any); ok {
                            var rr []Point
                            for _, p := range arr {
                                if vv, ok := p.([]any); ok && len(vv) >= 2 {
                                    lon := toFloat(vv[0]); lat := toFloat(vv[1])
                                    rr = append(rr, Point{Lat: lat, Lon: lon})
                                }
                            }
                            poly.Rings = append(poly.Rings, rr)
                        }
                    }
                }
                poly.BBox = computeBBox(poly)
                u.Polys = append(u.Polys, poly)
            }
        }
        return
    }
}

func computeBBox(p Polygon) [4]float64 {
    b := [4]float64{180, 90, -180, -90}
    for _, r := range p.Rings {
        for _, pt := range r {
            if pt.Lon < b[0] { b[0] = pt.Lon }
            if pt.Lat < b[1] { b[1] = pt.Lat }
            if pt.Lon > b[2] { b[2] = pt.Lon }
            if pt.Lat > b[3] { b[3] = pt.Lat }
        }
    }
    return b
}

func getStr(m map[string]any, k string) string {
    if v, ok := m[k].(string); ok { return v }
    return ""
}

func toFloat(v any) float64 {
    switch x := v.(type) {
    case float64:
        return x
    case float32:
        return float64(x)
    case int:
        return float64(x)
    case int64:
        return float64(x)
    default:
        return 0
    }
}

