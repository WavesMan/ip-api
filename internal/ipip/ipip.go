// 包 ipip：读取与遍历 IPIP IPDB 数据文件，提供 IPv4 叶子枚举以支持批量导入
// 背景：围绕二叉前缀树的紧凑存储结构实现，只暴露只读 Reader 接口，降低误用风险与实现复杂度。
package ipip

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"ip-api/internal/logger"
	"os"
	"sync/atomic"
)

type meta struct {
	Build     int64          `json:"build"`
	IPVersion uint16         `json:"ip_version"`
	Languages map[string]int `json:"languages"`
	NodeCount int            `json:"node_count"`
	TotalSize int            `json:"total_size"`
	Fields    []string       `json:"fields"`
}

// 文档注释：元信息结构（来自文件头部 JSON）
// 背景：描述数据版本、字段集合与节点数等加载所需信息；用于语言偏移与解析边界判断。
// 约束：字段列表与语言映射必须非空，否则视为文件不合法；TotalSize 用于强校验文件体积避免截断。

type Reader struct {
	fileSize  int
	nodeCount int
	v4offset  int
	meta      meta
	data      []byte
}

// 文档注释：数据读取器
// 背景：持有解析后的元信息与原始二进制数据片段；v4offset 为 IPv4 根节点偏移，通过前序遍历计算。
// 约束：只读，不暴露写入接口；内部偏移与索引均以大端字节序解析，错误边界检查严格返回 error。

var firstLeafLogged atomic.Bool

// 文档注释：打开并解析 IPDB 文件
// 参数：path 为文件路径；会执行尺寸校验、元信息 JSON 解码与数据段截取。
// 返回：Reader 指针；异常包含文件不存在、尺寸错误、JSON 不合法、总大小不匹配等场景。
// NOTE: v4offset 通过前 96 层节点读取确定根偏移，日志仅输出偏移与首叶样本，便于问题定位。
func Open(path string) (*Reader, error) {
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
	var m meta
	if err := json.Unmarshal(body[4:4+mlen], &m); err != nil {
		return nil, err
	}
	if len(m.Languages) == 0 || len(m.Fields) == 0 {
		return nil, errors.New("bad ipdb meta fields")
	}
	if size != (4 + mlen + m.TotalSize) {
		return nil, errors.New("bad ipdb total size")
	}
	r := &Reader{fileSize: size, nodeCount: m.NodeCount, meta: m, data: body[4+mlen:]}
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
		logger.L().Debug("ipip_v4offset", "offset", r.v4offset)
	}
	return r, nil
}

// 文档注释：读取节点指针
// 背景：节点存储为连续 8 字节，左右各 4 字节（BE）；index=0 读左指针，index=1 读右指针。
// 约束：越界返回原节点以避免 panic；调用方需结合深度与节点范围控制递归。
func (r *Reader) readNode(node, index int) int {
	off := node*8 + index*4
	if off+4 > len(r.data) {
		return node
	}
	return int(binary.BigEndian.Uint32(r.data[off : off+4]))
}

// 文档注释：解析叶子数据
// 背景：叶子节点以“长度(uint16 BE) + 数据”格式存储在数据段末尾；解码后返回原始字节切片。
// 返回：叶子原始数据；异常覆盖越界与长度不合法。
func (r *Reader) resolve(node int) ([]byte, error) {
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

type IPv4Leaf struct {
	Prefix uint32
	Length int
	Raw    []byte
}

// 文档注释：IPv4 前缀叶子
// 背景：Prefix 为位前缀累积，Length 为前缀长度（0–32），Raw 为对应位置的元数据原始字节串。
// 约束：调用方需自行按语言偏移与字段映射解析 Raw 内容。

// 文档注释：枚举 IPv4 叶子（DFS 前序遍历）
// 背景：从 v4offset 根开始深度优先遍历，遇到叶子节点时下发到通道；用于批量导入与并行解析。
// 参数：ch 为输出通道，调用方负责消费与关闭时机（函数内部不关闭）。
// 异常：解析叶子数据失败时返回 error；为避免阻塞，建议消费者使用足够大的缓冲或并发消费。
func (r *Reader) EnumerateIPv4(ch chan<- IPv4Leaf) error {
	var dfs func(node int, depth int, prefix uint32) error
	dfs = func(node int, depth int, prefix uint32) error {
		if node > r.nodeCount {
			raw, err := r.resolve(node)
			if err != nil {
				return err
			}
			if !firstLeafLogged.Load() {
				logger.L().Debug("ipip_leaf_sample", "prefix", int(prefix), "length", depth, "raw_len", len(raw))
				firstLeafLogged.Store(true)
			}
			ch <- IPv4Leaf{Prefix: prefix, Length: depth, Raw: raw}
			return nil
		}
		if depth >= 32 {
			return nil
		}
		left := r.readNode(node, 0)
		right := r.readNode(node, 1)
		// 左分支，bit 0
		if err := dfs(left, depth+1, prefix<<1); err != nil {
			return err
		}
		// 右分支，bit 1
		if err := dfs(right, depth+1, (prefix<<1)|1); err != nil {
			return err
		}
		return nil
	}
	return dfs(r.v4offset, 0, 0)
}
