// 包 localdb：树形步进式查询（直接读取 IPIP IPDB），避免范围 SQL 与内存常驻
package localdb

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"os"
)

// 文档注释：IPIP 文件头部元信息
// 背景：用于校验文件合法性并指导语言偏移与字段解析；缺少必要字段视为文件不合法。
type ipipMeta struct {
	Build     int64          `json:"build"`
	IPVersion uint16         `json:"ip_version"`
	Languages map[string]int `json:"languages"`
	NodeCount int            `json:"node_count"`
	TotalSize int            `json:"total_size"`
	Fields    []string       `json:"fields"`
}

// 文档注释：IPIP 前缀树只读结构
// 背景：持有数据段与节点计数，通过根偏移逐位下钻得到叶子；只读，避免并发写入复杂度。
type IPIPTree struct {
	nodeCount int
	v4offset  int
	meta      ipipMeta
	data      []byte
}

// 文档注释：打开并解析 IPDB 文件
// 背景：严格校验头部 JSON 与总尺寸，计算 IPv4 根偏移（前 96 层路径），为后续查找提供起点；失败直接返回错误。
// WARNING: 文件截断或元信息异常需尽快替换数据源；不尝试修复以避免错误数据污染。
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

// 文档注释：读取节点指针
// 背景：节点以 8 字节存储左右指针（BE）；越界返回原节点以保证查找路径稳健，避免 panic。
func (r *IPIPTree) readNode(node, index int) int {
	off := node*8 + index*4
	if off+4 > len(r.data) {
		return node
	}
	return int(binary.BigEndian.Uint32(r.data[off : off+4]))
}

// 文档注释：解析叶子数据
// 背景：叶子存储在数据段末尾，以长度 + 数据形式；解析失败返回错误，交由上层回退。
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

// 文档注释：计算语言偏移
// 背景：字段按语言起始下标组织；若目标语言缺失则回退到最小偏移以最大化可读字段。
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

// 文档注释：树形查找包装器（无内存常驻）
// 背景：仅持有只读树与语言偏移；每次查询按位下钻并解析叶子，避免建立范围索引或加载全集。
type IPIPCache struct {
	r   *IPIPTree
	off int
}

// 文档注释：构建树形查找缓存
// 背景：打开文件并计算语言偏移；失败时不影响主流程（由上层选择备用路径）。
func NewIPIPCache(path string, language string) (*IPIPCache, error) {
	r, err := OpenIPIP(path)
	if err != nil {
		return nil, err
	}
	return &IPIPCache{r: r, off: langOffset(r.meta, language)}, nil
}

// 文档注释：步进式前缀树查询
// 背景：从 IPv4 根偏移开始，按 32 位逐位选择左右分支；命中叶子后解析字段并返回地点信息。
// 约束：仅支持 IPv4；解析失败视为未命中；不依赖数据库，适合作为广覆盖的只读数据源。
func (c *IPIPCache) Lookup(ip string) (Location, bool) {
	var zero Location
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
	var l Location
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
