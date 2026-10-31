// Package ids 定义雪花 ID 的契约类型与进程级生成器。
// DB 无自增（sys_user.id 为 bigint 无 AUTO_INCREMENT），ID 由应用层生成；
// JSON 出网恒为十进制字符串（int64 超出 JS Number.MAX_SAFE_INTEGER）。
package ids

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/bwmarrin/snowflake"
)

// ID 雪花 ID 契约类型：MarshalJSON 输出带引号十进制字符串；UnmarshalJSON
// 接受带引号字符串与裸数字双形态（源 Jackson Long 双兼容语义），"null"→0。
type ID int64

// MarshalJSON 序列化为带引号的十进制字符串。
func (i ID) MarshalJSON() ([]byte, error) {
	return []byte(`"` + strconv.FormatInt(int64(i), 10) + `"`), nil
}

// UnmarshalJSON 兼容 "456" 与 456 双形态；null→0 不报错；非法值返回描述性 error。
func (i *ID) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" {
		*i = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid id %q", s)
	}
	*i = ID(v)
	return nil
}

// Node 雪花 ID 生成器封装。
type Node struct {
	node *snowflake.Node
}

// NewNode 按指定 workerId 创建生成器（n 取值 0..1023，越界返回 error）。
func NewNode(n int) (*Node, error) {
	node, err := snowflake.NewNode(int64(n))
	if err != nil {
		return nil, fmt.Errorf("create snowflake node %d: %w", n, err)
	}
	return &Node{node: node}, nil
}

// NewID 生成雪花 ID（内置 monotonic 防钟漂与并发序列管理）。
func (n *Node) NewID() ID {
	return ID(n.node.Generate().Int64())
}

var (
	mu   sync.Mutex
	dflt *Node
)

// SetDefault 装配进程级单例节点（main 启动时按配置调用一次）。
func SetDefault(n *Node) {
	mu.Lock()
	defer mu.Unlock()
	dflt = n
}

// Default 返回进程默认节点；未显式装配时惰性创建 workerId=1（单进程形态安全，
// 见研究 Assumption A4）。多实例部署前应改为显式装配。
func Default() *Node {
	mu.Lock()
	defer mu.Unlock()
	if dflt == nil {
		n, err := NewNode(1)
		if err != nil {
			// workerId=1 恒合法（0..1023），此处不可达
			panic(fmt.Sprintf("ids: lazy default node: %v", err))
		}
		dflt = n
	}
	return dflt
}

// NewID 用进程默认节点生成雪花 ID。
func NewID() ID {
	return Default().NewID()
}
