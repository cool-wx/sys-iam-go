package ids_test

import (
	"encoding/json"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

func TestIDMarshalJSON(t *testing.T) {
	b, err := json.Marshal(ids.ID(456))
	require.NoError(t, err)
	assert.Equal(t, `"456"`, string(b), "ID 必须序列化为带引号十进制字符串")
}

func TestIDUnmarshalJSONDualForm(t *testing.T) {
	var fromStr ids.ID
	require.NoError(t, json.Unmarshal([]byte(`"456"`), &fromStr))
	assert.Equal(t, ids.ID(456), fromStr, "带引号字符串形态")

	var fromNum ids.ID
	require.NoError(t, json.Unmarshal([]byte(`456`), &fromNum))
	assert.Equal(t, ids.ID(456), fromNum, "裸数字形态")
}

func TestIDUnmarshalJSONNull(t *testing.T) {
	var v ids.ID = 123
	require.NoError(t, v.UnmarshalJSON([]byte(`null`)))
	assert.Equal(t, ids.ID(0), v, "null → 0 且不报错")
}

func TestIDUnmarshalJSONInvalid(t *testing.T) {
	var v ids.ID
	err := v.UnmarshalJSON([]byte(`"not-a-number"`))
	require.Error(t, err, "非数字串必须返回描述性 error")
	assert.Contains(t, err.Error(), "invalid id")
}

func TestSnowflakeConcurrentUniqueness(t *testing.T) {
	// 并发唯一性：8 goroutine × 2000 次 = 16000 全部唯一且 > 2^53
	node, err := ids.NewNode(1)
	require.NoError(t, err)

	const workers = 8
	const perWorker = 2000
	var (
		mu   sync.Mutex
		seen = make(map[int64]struct{}, workers*perWorker)
		wg   sync.WaitGroup
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				id := node.NewID()
				mu.Lock()
				seen[int64(id)] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Len(t, seen, workers*perWorker, "16000 个并发 ID 必须零重复")
	for id := range seen {
		if id <= int64(1)<<53 {
			t.Fatalf("ID %d 未超过 2^53（JS Number.MAX_SAFE_INTEGER）", id)
		}
	}
}

func TestSnowflakeLargeIDStringRoundTrip(t *testing.T) {
	// 超过 2^53 的真实雪花值经 JSON 字符串化后无损往返
	node, err := ids.NewNode(1)
	require.NoError(t, err)
	raw, err := json.Marshal(node.NewID())
	require.NoError(t, err)

	var back ids.ID
	require.NoError(t, json.Unmarshal(raw, &back))
	_, parseErr := strconv.ParseInt(string(trimQuotes(raw)), 10, 64)
	require.NoError(t, parseErr)
	assert.Equal(t, string(trimQuotes(raw)), strconv.FormatInt(int64(back), 10), "字符串化往返无损")
}

func trimQuotes(b []byte) []byte {
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return []byte(s[1 : len(s)-1])
	}
	return b
}
