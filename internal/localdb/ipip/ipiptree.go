package ipip

import (
    "encoding/binary"
    "encoding/json"
    "errors"
    "ip-api/internal/localdb"
    "net"
    "os"
)

type ipipMeta struct {
    Build     int64          `json:"build"`
    IPVersion uint16         `json:"ip_version"`
    Languages map[string]int `json:"languages"`
    NodeCount int            `json:"node_count"`
    TotalSize int            `json:"total_size"`
    Fields    []string       `json:"fields"`
}

type IPIPTree struct {
    nodeCount int
    v4offset  int
    meta      ipipMeta
    data      []byte
}

func OpenIPIP(path string) (*IPIPTree, error) {
    fi, err := os.Stat(path)
    if err != nil {
        return nil, err
    }
    size := int(fi.Size())
    if size < 4 {
        return nil, errors.New("bad ipdb size")
    }
    body, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    mlen := int(binary.BigEndian.Uint32(body[0:4]))
    if size < 4+mlen {
        return nil, errors.New("bad ipdb meta")
    }
    var m ipipMeta
    if err := json.Unmarshal(body[4:4+mlen], &m); err != nil {
        return nil, err
    }
    if len(m.Languages) == 0 || len(m.Fields) == 0 {
        return nil, errors.New("bad ipdb meta fields")
    }
    if size != (4 + mlen + m.TotalSize) {
        return nil, errors.New("bad ipdb total size")
    }
    r := &IPIPTree{nodeCount: m.NodeCount, meta: m, data: body[4+mlen:]}
    if r.v4offset == 0 {
        node := 0
        for i := 0; i < 96 && node < r.nodeCount; i++ {
            if i >= 80 {
                node = r.readNode(node, 1)
            } else {
                node = r.readNode(node, 0)
            }
        }
        r.v4offset = node
    }
    return r, nil
}

func (r *IPIPTree) readNode(node, index int) int {
    off := node*8 + index*4
    if off+4 > len(r.data) {
        return node
    }
    return int(binary.BigEndian.Uint32(r.data[off : off+4]))
}

func (r *IPIPTree) resolve(node int) ([]byte, error) {
    resolved := node - r.nodeCount + r.nodeCount*8
    if resolved >= len(r.data) {
        return nil, errors.New("resolve out of range")
    }
    size := int(binary.BigEndian.Uint16(r.data[resolved : resolved+2]))
    if (resolved + 2 + size) > len(r.data) {
        return nil, errors.New("resolve size")
    }
    return r.data[resolved+2 : resolved+2+size], nil
}

func langOffset(m ipipMeta, language string) int {
    if off, ok := m.Languages[language]; ok {
        return off
    }
    have := false
    min := 0
    for _, v := range m.Languages {
        if !have || v < min {
            min = v
            have = true
        }
    }
    return min
}

type IPIPCache struct {
    r   *IPIPTree
    off int
}

func NewIPIPCache(path string, language string) (*IPIPCache, error) {
    r, err := OpenIPIP(path)
    if err != nil {
        return nil, err
    }
    return &IPIPCache{r: r, off: langOffset(r.meta, language)}, nil
}

func (c *IPIPCache) Lookup(ip string) (localdb.Location, bool) {
    var zero localdb.Location
    p := net.ParseIP(ip)
    if p == nil || p.To4() == nil {
        return zero, false
    }
    v := p.To4()
    node := c.r.v4offset
    for i := 0; i < 32; i++ {
        b := (v[i/8] >> uint(7-(i%8))) & 1
        node = c.r.readNode(node, int(b))
        if node > c.r.nodeCount {
            break
        }
    }
    if node <= c.r.nodeCount {
        return zero, false
    }
    raw, err := c.r.resolve(node)
    if err != nil {
        return zero, false
    }
    fields := string(raw)
    parts := make([]string, 0, len(c.r.meta.Fields))
    start := 0
    for i := 0; i < len(fields); i++ {
        if fields[i] == '\t' {
            parts = append(parts, fields[start:i])
            start = i + 1
        }
    }
    parts = append(parts, fields[start:])
    begin := c.off
    end := c.off + len(c.r.meta.Fields)
    if begin < 0 {
        begin = 0
    }
    if end > len(parts) {
        end = len(parts)
    }
    if begin >= end {
        return zero, false
    }
    seg := parts[begin:end]
    var l localdb.Location
    for i, f := range c.r.meta.Fields {
        if i >= len(seg) {
            break
        }
        switch f {
        case "country_name":
            l.Country = seg[i]
        case "region_name":
            l.Region = seg[i]
        case "province_name":
            l.Province = seg[i]
        case "city_name":
            l.City = seg[i]
        }
    }
    return l, true
}
