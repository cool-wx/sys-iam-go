package storage_test

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/internal/storage"
)

func TestLocalStorageRoundtrip(t *testing.T) {
	// 全语义往返（FILE-05 local 主链，免容器）：Put 字节 → GetStream 读出一致 +
	// size 正确 → Delete → 再 GetStream 报错。
	ctx := context.Background()
	s := storage.NewLocalStorage(t.TempDir())
	content := []byte("local storage roundtrip payload 你好")
	key := "6f0d5c1e-0000-4000-8000-000000000001_report.txt"

	require.NoError(t, s.Put(ctx, key, bytes.NewReader(content), int64(len(content)), "text/plain"))

	r, size, err := s.GetStream(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, int64(len(content)), size, "GetStream 第二返回值必须为对象字节数（Content-Length 供源，FILE-02）")
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	assert.Equal(t, content, got, "读出内容必须与写入逐字节一致")

	require.NoError(t, s.Delete(ctx, key))
	_, _, err = s.GetStream(ctx, key)
	require.Error(t, err, "删除后 GetStream 必须报错（对象已不存在）")
}

func TestLocalStorageDeleteMissingTolerant(t *testing.T) {
	// Delete 不存在 key 返回 nil（FILE-03 宽容语义基座——对齐 S3 RemoveObject
	// 幂等语义，删除链存储侧"不存在"不算失败）。
	ctx := context.Background()
	s := storage.NewLocalStorage(t.TempDir())
	assert.NoError(t, s.Delete(ctx, "00000000-0000-4000-8000-000000000002_never-existed.bin"),
		"删除不存在对象必须幂等宽容返回 nil")
}

func TestLocalStorageKeyEscapeDefense(t *testing.T) {
	// T-4-02 防御纵深（prohibition：key 拼 path 不得直接拼接）：含路径穿越片段
	// 的 key 经 resolveKey 清洗+校验后，落点必须收敛在 root 子树内——逃逸直证：
	// 自建 sandbox 完全独享（root = sandbox/root），Put 后 sandbox 顶层必须只有
	// root 一个目录，穿越落点不可能出现在 root 之外。
	ctx := context.Background()
	sandbox, err := os.MkdirTemp("", "iam-escape-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(sandbox) })
	root := filepath.Join(sandbox, "root")
	s := storage.NewLocalStorage(root)
	content := []byte("escape-attempt")

	escapeKeys := []string{
		"../../outside.txt",       // 直接回溯两层
		"a/../../b/c.txt",         // 混合前进+回溯
		"....//....//escape.bin",  // "...." 非穿越但 // 混淆形态
		"/absolutely/outside.txt", // 绝对路径形态
	}
	accepted := 0
	for _, key := range escapeKeys {
		err := s.Put(ctx, key, bytes.NewReader(content), int64(len(content)), "text/plain")
		if err != nil {
			continue // 被 resolveKey 拒绝——合法防御形态
		}
		accepted++
		// 未被拒绝则同一 key 必须可从存储读回（写读同路径，无不对称逃逸面）。
		r, _, err := s.GetStream(ctx, key)
		require.NoError(t, err, "key %q 写入后必须同路径可读", key)
		got, err := io.ReadAll(r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		assert.Equal(t, content, got)
	}
	require.Equal(t, len(escapeKeys), accepted, "四个穿越 key 均应被清洗吸收（Clean 语义）非拒绝")

	// 逃逸直证：sandbox 顶层只有 root 一个条目；全部 4 文件都在 root 子树内。
	entries, err := os.ReadDir(sandbox)
	require.NoError(t, err)
	require.Len(t, entries, 1, "sandbox 顶层必须只有 root 一个条目（穿越片段被清洗吸收，无逃逸落点）")
	assert.Equal(t, "root", entries[0].Name())
	var files int
	require.NoError(t, filepath.Walk(sandbox, func(_ string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files++
		}
		return nil
	}))
	assert.Equal(t, accepted, files, "sandbox 全树文件数必须恰为被接受的 key 数（零逃逸零丢失）")
}
