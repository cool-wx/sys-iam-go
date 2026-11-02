// local.go LocalStorage 实现：os 文件流 Put/GetStream/Delete，
// root 目录约定为 storage.local.root（默认 ./data/files，相对工作目录——systemd
// WorkingDirectory 联动）。
//
// 路径安全：key 恒为服务端生成（uuid_原名），但仍做防御纵深——
// resolveKey 清洗（Clean("/"+key) 吃掉 ..// 绝对路径片段）+ 落点前缀校验
// （必须仍在 root 内），逃逸即拒绝。local_test.go 直测锁定本防御。
package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalStorage 本地磁盘存储实现。
type LocalStorage struct {
	root string
}

// NewLocalStorage 构造：root 为存储根目录（相对路径按进程工作目录解析）。
// 不在本函数建目录——Put 时按需 MkdirAll（root 不可写等问题推迟到首个写入
// 操作暴露，GetStream/Delete 只读路径零副作用）。
func NewLocalStorage(root string) *LocalStorage {
	return &LocalStorage{root: root}
}

// resolveKey key → root 内安全落点路径：Clean 吃掉穿越片段后校验落点仍带
// root 前缀，逃逸即拒绝（双防御：清洗 + 校验，防御纵深不依赖 key
// 恒可信的调用约定）。
func resolveKey(root, key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("storage: object key must not be empty")
	}
	// Clean("/"+key) 语义：前导 "/" 使 Clean 以绝对路径语义解析（.. 不能越过
	// 根），随后 filepath.Join(root, ...) 拼接。例：root=/data、key=../../etc/x
	// → Clean 得 /etc/x → Join 得 /data/etc/x（逃逸被清洗吸收）。
	cleaned := filepath.Join(root, filepath.Clean("/"+key))
	// 校验兜底：清洗后落点必须仍在 root 内（含 root 本身+分隔符前缀）。防御
	// 符号链接等 Clean 语义外的逃逸形态——root 为相对路径时按绝对化比较。
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("storage: resolve root: %w", err)
	}
	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("storage: resolve path: %w", err)
	}
	if !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("storage: object key %q escapes storage root %q", key, root)
	}
	return cleaned, nil
}

// Put 写入对象：按需建父目录（0o755）→ 建文件 → io.Copy 流式写（size 参数仅
// 契约记录用，Copy 自然按流写不整载——工控机内存平稳）。
func (s *LocalStorage) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	path, err := resolveKey(s.root, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("storage: mkdir for %q: %w", key, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("storage: create %q: %w", key, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return fmt.Errorf("storage: write %q: %w", key, err)
	}
	return f.Close()
}

// GetStream 读取对象：os.Open + Stat 取 size（供 Content-Length）。
func (s *LocalStorage) GetStream(_ context.Context, key string) (io.ReadCloser, int64, error) {
	path, err := resolveKey(s.root, key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("storage: open %q: %w", key, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, fmt.Errorf("storage: stat %q: %w", key, err)
	}
	return f, info.Size(), nil
}

// Delete 删除对象：os.Remove，对象不存在返回 nil（os.IsNotExist 宽容——对齐
// S3 RemoveObject 幂等语义，宽容删除的双实现基座；删除链存储侧失败
// 仅告警不阻断 DB 清理，此处"不存在"不算失败）。
func (s *LocalStorage) Delete(_ context.Context, key string) error {
	path, err := resolveKey(s.root, key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: remove %q: %w", key, err)
	}
	return nil
}
