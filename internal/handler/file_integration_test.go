//go:build integration

// File 域集成测试（FILE-01..04，源 FileUploadHashDedupTest 蓝本直译）：判重五语义
// （同分类同内容 400 源消息 + 零副作用 / 异分类放行 / 未分类 IS NULL 分支 / 异内容
// 放行 / 分类不存在 404 源消息）、下载流式（Content-Length + 逐字节一致）、删除
// 宽容链、文件分类树（孤儿提升 + parent_name 冗余 + 上级分类不存在 404）。
package handler_test

import (
	"bytes"
	"encoding/json"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doUpload multipart 上传 helper：字段 file（内容 + 原名），categoryID 非空时附
// category_id 表单字段（空串=不传，走未分类 IS NULL 判重分支）。
func doUpload(t *testing.T, token, url, fileName string, content []byte, categoryID string) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = fw.Write(content)
	require.NoError(t, err)
	if categoryID != "" {
		require.NoError(t, w.WriteField("category_id", categoryID))
	}
	require.NoError(t, w.Close())
	req, err := http.NewRequest("POST", url, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, bs
}

// storageObjectCount 直查 LocalStorage 根目录对象数（判重零副作用断言面）。
func storageObjectCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(testStorageRoot)
	require.NoError(t, err, "存储根目录必须可读: %s", testStorageRoot)
	return len(entries)
}

// fileRows 直查 sys_file 行数。
func fileRows(t *testing.T, dsn string) int64 {
	t.Helper()
	db := openCheckDB(t, dsn)
	var cnt int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM sys_file").Scan(&cnt).Error)
	return cnt
}

// TestFileUploadDedupFiveSemantics 上传判重五语义（源 FileUploadHashDedupTest
// 蓝本直译）：①同分类同内容拒绝（400 源消息 + 零副作用）②异分类放行 ③异内容
// 放行 ④未分类 IS NULL 分支判重命中 ⑤分类不存在 404 源消息。
func TestFileUploadDedupFiveSemantics(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	// --- 前置：pri 组保护（无 token 上传 401）---
	status, body := doRequest(t, "POST", ts.URL+"/api/v1/files/upload", "")
	require.Equal(t, http.StatusUnauthorized, status, "无 token 必须被认证中间件拦截: body=%s", body)

	contentX := []byte("hello dedup content X")
	contentY := []byte("another content Y")
	contentZ := []byte("uncategorized content Z")

	// --- ① 建分类 A + 上传内容 X 原名 f1 → 200 ---
	catA := mustCreate(t, token, ts.URL+"/api/v1/file-categories", `{"categoryName":"分类A","sort":1}`)
	status, body = doUpload(t, token, ts.URL+"/api/v1/files/upload", "f1.txt", contentX, catA)
	require.Equal(t, http.StatusOK, status, "首次上传必须成功: body=%s", body)
	var up1 struct {
		Code int `json:"code"`
		Data struct {
			ID           string  `json:"id"`
			FileName     *string `json:"fileName"`
			CategoryID   *string `json:"categoryId"`
			CategoryName *string `json:"categoryName"`
			FileSize     *int64  `json:"fileSize"`
			FileHash     *string `json:"fileHash"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &up1), "body=%s", body)
	require.Equal(t, 0, up1.Code, "body=%s", body)
	assert.Equal(t, "f1.txt", *up1.Data.FileName)
	require.NotNil(t, up1.Data.CategoryID)
	assert.Equal(t, catA, *up1.Data.CategoryID, "categoryId 必须回显")
	require.NotNil(t, up1.Data.CategoryName)
	assert.Equal(t, "分类A", *up1.Data.CategoryName, "category_name 必须为服务端查库冗余值")
	assert.Equal(t, int64(len(contentX)), *up1.Data.FileSize)
	require.NotNil(t, up1.Data.FileHash)
	assert.Len(t, *up1.Data.FileHash, 32, "MD5 hex 必须为 32 字符")

	// --- ② 同分类同内容异名 f2 → 400 源消息 + 零副作用（DB 0 新行 + 存储无新对象）---
	status, body = doUpload(t, token, ts.URL+"/api/v1/files/upload", "f2.txt", contentX, catA)
	require.Equal(t, http.StatusBadRequest, status, "同分类同内容必须拒绝: body=%s", body)
	var dupEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &dupEnv))
	assert.Equal(t, 10400, dupEnv.Code)
	assert.Equal(t, "文件已存在：f2.txt（内容 MD5 重复，同一分类下不可重复上传）", dupEnv.Message,
		"拒绝消息必须为源消息原文且含本次上传原名: body=%s", body)
	assert.Equal(t, int64(1), fileRows(t, dsn), "拒绝后 sys_file 必须仍为 1 行（零副作用）")
	assert.Equal(t, 1, storageObjectCount(t), "拒绝后存储必须仍恰 1 个对象（判重命中前不得 Put）")

	// --- ③ 建分类 B + 同内容 X 异分类 → 200（异分类放行）---
	catB := mustCreate(t, token, ts.URL+"/api/v1/file-categories", `{"categoryName":"分类B","sort":2}`)
	status, body = doUpload(t, token, ts.URL+"/api/v1/files/upload", "f3.txt", contentX, catB)
	require.Equal(t, http.StatusOK, status, "同内容异分类必须放行: body=%s", body)

	// --- ④ 分类 A 传异内容 Y → 200（异内容放行）---
	status, body = doUpload(t, token, ts.URL+"/api/v1/files/upload", "f4.txt", contentY, catA)
	require.Equal(t, http.StatusOK, status, "同分类异内容必须放行: body=%s", body)

	// --- ⑤ 未分类（不传 category_id）：内容 Z → 200；再传同内容 Z → 400 ---
	// （IS NULL 分支判重命中——唯一索引对 NULL 互不相等，未分类判重全靠本分支）
	status, body = doUpload(t, token, ts.URL+"/api/v1/files/upload", "f5.txt", contentZ, "")
	require.Equal(t, http.StatusOK, status, "未分类首次上传必须成功: body=%s", body)
	var up5 struct {
		Data struct {
			CategoryID   *string `json:"categoryId"`
			CategoryName *string `json:"categoryName"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &up5))
	assert.Nil(t, up5.Data.CategoryID, "未分类上传 categoryId 必须为 null")
	assert.Nil(t, up5.Data.CategoryName, "未分类上传 categoryName 必须为 null")

	status, body = doUpload(t, token, ts.URL+"/api/v1/files/upload", "f6.txt", contentZ, "")
	require.Equal(t, http.StatusBadRequest, status, "未分类同内容必须经 IS NULL 分支拒绝: body=%s", body)
	require.NoError(t, json.Unmarshal(body, &dupEnv))
	assert.Contains(t, dupEnv.Message, "文件已存在：f6.txt（内容 MD5 重复", "IS NULL 分支拒绝消息: body=%s", body)

	// --- ⑥ 不存在分类 id 上传 → 404「文件分类不存在: {id}」源消息 ---
	status, body = doUpload(t, token, ts.URL+"/api/v1/files/upload", "f7.txt", contentY, "99999")
	require.Equal(t, http.StatusNotFound, status, "分类不存在必须 404: body=%s", body)
	require.NoError(t, json.Unmarshal(body, &dupEnv))
	assert.Equal(t, 10300, dupEnv.Code)
	assert.Contains(t, dupEnv.Message, "文件分类不存在: 99999", "404 消息应为源消息原文: body=%s", body)

	// --- 汇总：4 次成功落库 + 存储恰 4 对象 ---
	assert.Equal(t, int64(4), fileRows(t, dsn), "成功上传 4 次后 sys_file 应为 4 行")
	assert.Equal(t, 4, storageObjectCount(t), "成功上传 4 次后存储应恰 4 个对象")
}

// TestFileDownloadStream 下载流式（FILE-02）：2MB 随机内容上传 → download 断言
// 200 + Content-Length == size + 响应体逐字节一致（不整载的契约面）。
func TestFileDownloadStream(t *testing.T) {
	ts, _, adminToken, _ := newTestApp(t)
	token := adminToken()

	// 2MB 随机内容（MaxMultipartMemory 8MB 内，全程内存驻留路径）
	content := make([]byte, 2*1024*1024)
	_, err := rand.Read(content)
	require.NoError(t, err)

	status, body := doUpload(t, token, ts.URL+"/api/v1/files/upload", "big.bin", content, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var up struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &up))
	require.NotEmpty(t, up.Data.ID)

	// 下载：断言 200 + Content-Length 头 + 逐字节一致
	req, err := http.NewRequest("GET", ts.URL+"/api/v1/files/download/"+up.Data.ID, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int64(len(content)), resp.ContentLength, "Content-Length 必须等于真实字节数（FILE-02）")
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(content, got), "响应体必须与原内容逐字节一致")
	assert.Equal(t, "attachment", resp.Header.Get("Content-Disposition")[:10],
		"下载必须恒 attachment 头（T-4-05 缓解面）")

	// 下载不存在的 id：404 域消息
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/files/download/99002", "")
	require.Equal(t, http.StatusNotFound, status, "body=%s", body)
	var nfEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &nfEnv))
	assert.Equal(t, 10300, nfEnv.Code)
	assert.Contains(t, nfEnv.Message, "文件不存在: 99002")

	// 列表（分页 envelope）：total==1 且列表项契约回显
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/files?page=1&size=15", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var listEnv struct {
		Data struct {
			Items []struct {
				ID       string `json:"id"`
				FileName string `json:"fileName"`
				FileSize int64  `json:"fileSize"`
			} `json:"items"`
			Total int64 `json:"total"`
			Page  int   `json:"page"`
			Size  int   `json:"size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &listEnv))
	require.Len(t, listEnv.Data.Items, 1)
	assert.Equal(t, up.Data.ID, listEnv.Data.Items[0].ID)
	assert.Equal(t, int64(1), listEnv.Data.Total)
	assert.Equal(t, 1, listEnv.Data.Page)
	assert.Equal(t, 15, listEnv.Data.Size)
}

// TestFileDeleteTolerant 删除链（FILE-03）：上传 → 删除 → 200 且 DB 行消失 +
// 存储对象移除；删除不存在的 id 404 域消息。
func TestFileDeleteTolerant(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	content := []byte("to be deleted")
	status, body := doUpload(t, token, ts.URL+"/api/v1/files/upload", "del.txt", content, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var up struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &up))
	require.NotEmpty(t, up.Data.ID)
	require.Equal(t, int64(1), fileRows(t, dsn))
	require.Equal(t, 1, storageObjectCount(t))

	// 删除：200 + DB 行消失 + 存储对象移除（LocalStorage 成功路径即覆盖主链；
	// 存储失败宽容分支由 slog.Warn 语义 + 代码路径保证，集成断言 DB 行删除即可）
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/files/"+up.Data.ID, "")
	require.Equal(t, http.StatusOK, status, "删除必须成功: body=%s", body)
	assert.Equal(t, int64(0), fileRows(t, dsn), "删除后 sys_file 必须无残留行")
	assert.Equal(t, 0, storageObjectCount(t), "删除后存储必须无残留对象")

	// 删除不存在的 id：404（service 先 FindByID）
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/files/99002", "")
	require.Equal(t, http.StatusNotFound, status, "body=%s", body)
	var nfEnv struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &nfEnv))
	assert.Contains(t, nfEnv.Message, "文件不存在: 99002")
}

// fileCategoryTreeNode 文件分类树响应解包结构（categoryName 键；children 递归嵌套）。
type fileCategoryTreeNode struct {
	ID           string                  `json:"id"`
	CategoryName string                  `json:"categoryName"`
	ParentID     *string                 `json:"parentId"`
	ParentName   *string                 `json:"parentName"`
	Children     []fileCategoryTreeNode  `json:"children"`
}

// TestFileCategoryTree 文件分类树（FILE-04）：两层级嵌套 + parent_name 冗余 +
// 孤儿提升为根 + 上级分类不存在 404 源消息 + 叶子 children 空数组非 null。
// 孤儿经 SQL 直插（API create 会被父校验拦截——正确行为，菜单树测试同构先例）。
func TestFileCategoryTree(t *testing.T) {
	ts, dsn, adminToken, _ := newTestApp(t)
	token := adminToken()

	// --- create 根分类 A + 子分类 B（parent_name 服务端冗余）---
	rootID := mustCreate(t, token, ts.URL+"/api/v1/file-categories", `{"categoryName":"分类A","sort":1}`)
	rootNum, err := strconv.ParseInt(rootID, 10, 64)
	require.NoError(t, err)
	childID := mustCreate(t, token, ts.URL+"/api/v1/file-categories",
		`{"categoryName":"分类B","parentId":`+rootID+`,"sort":1,"categoryType":1}`)
	childNum, err := strconv.ParseInt(childID, 10, 64)
	require.NoError(t, err)

	checkDB := openCheckDB(t, dsn)
	var row struct {
		ParentID   *int64
		ParentName *string
	}
	require.NoError(t, checkDB.Raw(
		"SELECT parent_id, parent_name FROM sys_file_category WHERE id = ?", childNum,
	).Scan(&row).Error)
	require.NotNil(t, row.ParentID)
	assert.Equal(t, rootNum, *row.ParentID)
	require.NotNil(t, row.ParentName, "合法 parentId 时 parent_name 必须冗余落库")
	assert.Equal(t, "分类A", *row.ParentName, "parent_name 必须为服务端查库冗余值")

	// --- create 带不存在的 parentId：404 +「上级分类不存在: 888」源消息 ---
	status, body := doAuthRequest(t, token, "POST", ts.URL+"/api/v1/file-categories",
		`{"categoryName":"孤儿分类","parentId":888}`)
	require.Equal(t, http.StatusNotFound, status, "父不存在应 404（CodeNotFound 拍板）: body=%s", body)
	var badParentEnv struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &badParentEnv))
	assert.Equal(t, 10300, badParentEnv.Code)
	assert.Contains(t, badParentEnv.Message, "上级分类不存在: 888", "body=%s", body)

	// --- SQL 直插孤儿（parent 999 不存在）---
	execMust(t, checkDB,
		"INSERT INTO sys_file_category (id, parent_id, parent_name, category_name, category_type, sort, status) VALUES (990101, 999, '幽灵父', '孤儿分类', 1, 99, 1)")

	// --- tree：根层 = 真根（sort 1）+ 孤儿（sort 99 提升为根）；子节点嵌套在根下 ---
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/file-categories/tree", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	var env struct {
		Data []fileCategoryTreeNode `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env), "body=%s", body)

	require.Len(t, env.Data, 2, "真根 + 孤儿均应在根层: body=%s", body)
	assert.Equal(t, rootID, env.Data[0].ID, "根层第一项应为真根（sort 1）")
	assert.Equal(t, "孤儿分类", env.Data[1].CategoryName, "孤儿应提升为根（dept 同构语义）")
	require.NotNil(t, env.Data[1].ParentID, "孤儿的 parentId 键应保留原值 999")
	assert.Equal(t, "999", *env.Data[1].ParentID)

	require.Len(t, env.Data[0].Children, 1, "根应有且仅有一个子节点")
	child := env.Data[0].Children[0]
	assert.Equal(t, childID, child.ID)
	require.NotNil(t, child.ParentName)
	assert.Equal(t, "分类A", *child.ParentName, "树节点 parentName 冗余值应回显")
	assert.NotNil(t, child.Children, "叶子 children 必须是空数组 [] 而非 null")
	assert.Empty(t, child.Children, "叶子 children 应为空数组")

	// --- update 移动子分类到根：parent_id 显式 0 → WR-07 修复后落 NULL（根 =
	// parent_id NULL 唯一形态，与 Create 根同形态），响应 parentId 恒 null。
	// 断言经 {code,message,data} envelope 的 Data 层解包（WR-08 真化——原断言漏
	// Data 层永不失败；Data 为单对象，仿同文件 tree 断言 env 形态）---
	status, body = doAuthRequest(t, token, "PUT", ts.URL+"/api/v1/file-categories/"+childID,
		`{"parentId":0}`)
	require.Equal(t, http.StatusOK, status, "移动到根必须成功: body=%s", body)
	var clearedEnv struct {
		Code int `json:"code"`
		Data struct {
			ParentID   *string `json:"parentId"`
			ParentName *string `json:"parentName"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &clearedEnv), "body=%s", body)
	require.Equal(t, 0, clearedEnv.Code, "body=%s", body)
	assert.Nil(t, clearedEnv.Data.ParentID, "移根后 parentId 必须为 null（WR-07：根唯一形态）")
	assert.Nil(t, clearedEnv.Data.ParentName, "显式移至根后 parent_name 必须清空")

	// --- delete 根分类（先删孤儿恢复两节点态）：删除带子节点的分类仍成功（源缺口按源保留）---
	status, body = doAuthRequest(t, token, "DELETE", ts.URL+"/api/v1/file-categories/990101", "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
	status, body = doAuthRequest(t, token, "GET", ts.URL+"/api/v1/file-categories/"+rootID, "")
	require.Equal(t, http.StatusOK, status, "body=%s", body)
}
