package revgeo

// 文档注释：轻量 geohash 编码（base32）
// 背景：用于缓存键与网格近邻探测；避免引入外部库，保留精度到 6 字符约 1.2km。
// 约束：仅用于缓存与近邻网格枚举，不做行政区映射；近邻返回 8 邻域。
var base32 = []rune("0123456789bcdefghjkmnpqrstuvwxyz")

func encodeGeohash(lat, lon float64, precision int) string {
    latInt := []float64{-90, 90}
    lonInt := []float64{-180, 180}
    bits := []int{16, 8, 4, 2, 1}
    bit := 0
    ch := 0
    even := true
    out := make([]rune, 0, precision)
    for len(out) < precision {
        if even {
            mid := (lonInt[0] + lonInt[1]) / 2
            if lon >= mid { ch |= bits[bit]; lonInt[0] = mid } else { lonInt[1] = mid }
        } else {
            mid := (latInt[0] + latInt[1]) / 2
            if lat >= mid { ch |= bits[bit]; latInt[0] = mid } else { latInt[1] = mid }
        }
        even = !even
        if bit < 4 { bit++ } else { out = append(out, base32[ch]); bit = 0; ch = 0 }
    }
    return string(out)
}

