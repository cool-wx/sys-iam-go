// Package service 文件域业务逻辑（FILE-01/02/03）：上传 MD5 判重五语义（源
// FileUploadHashDedupTest 蓝本）+ 流式下载 + 存储宽容删除 + 分页列表。
//
// 上传链顺序（D-12 语义不变，实现用 multipart.File 的 Seek(0) 重放替代研究期
// TeeReader 表述——04-RESEARCH Implementation Guidance 建议）：分类校验 → 流式算
// MD5 → 判重（拒绝即返回，此刻存储/DB 零副作用）→ Seek(0) 重放流 → 事务内
// Create + storage.Put（源 save DB → 上传存储同序且 @Transactional 含存储调用，
// Put 失败 DB 回滚——横切发现 6）。
//
// 删除链（FILE-03 宽容语义，源 FileApplicationService:112-122 逐字）：存储端删除
// 失败仅 slog.Warn 留痕（「S3对象删除失败，继续删除数据库记录: {filePath}」），
// 继续删 DB 记录——存储失败绝不阻断 DB 清理。
package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/internal/storage"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/paging"
)

// FileService 文件域服务（依赖 FileStorage 抽象——同一套代码对 local/s3 后端
// 无差别，FILE-05；db 供上传链显式事务——app.go SkipDefaultTransaction=true）。
type FileService struct {
	db      *gorm.DB
	repo    *repository.FileInfoRepo
	catRepo *repository.FileCategoryRepo
	storage storage.FileStorage
}

// NewFileService 构造。
func NewFileService(db *gorm.DB, repo *repository.FileInfoRepo, catRepo *repository.FileCategoryRepo, fileStorage storage.FileStorage) *FileService {
	return &FileService{db: db, repo: repo, catRepo: catRepo, storage: fileStorage}
}

// Upload 上传文件（FILE-01 判重五语义）：handler 已解析 multipart FileHeader 与
// category_id 表单字段（空串=未分类）。返回落库后的文件信息契约。
//
// 零副作用保证（prohibition）：判重命中前不触碰存储与 DB——拒绝时 sys_file 零
// 新行、存储无对象（D-12 拒绝零副作用语义）。
func (s *FileService) Upload(ctx context.Context, fh *multipart.FileHeader, categoryIDRaw string) (*dto.FileInfoResp, error) {
	// (a) 打开上传流（multipart.File：小文件内存 bytes.Reader、大文件临时文件
	// *os.File——二者均实现 io.Seeker，RSS 平稳；MaxMultipartMemory 阈值落盘）
	f, err := fh.Open()
	if err != nil {
		return nil, errcode.New(errcode.CodeInvalidParam, "打开上传文件失败")
	}
	defer f.Close()

	// (b) categoryId 可选：非空解析 int64 且校验分类存在（缺失 → 404「文件分类
	// 不存在: {id}」源消息原文 FileApplicationService:65）
	var categoryID *int64
	var categoryName *string
	if categoryIDRaw != "" {
		id, parseErr := strconv.ParseInt(categoryIDRaw, 10, 64)
		if parseErr != nil {
			return nil, errcode.New(errcode.CodeInvalidParam, "无效的文件分类 ID")
		}
		cat, catErr := s.catRepo.FindByID(ctx, id)
		if catErr != nil {
			if errors.Is(catErr, errcode.ErrNotFound) {
				return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("文件分类不存在: %d", id))
			}
			return nil, catErr
		}
		categoryID = &id
		categoryName = cat.CategoryName // category_name 冗余（源 add 落库形态）
	}

	// (c) 流式算 MD5（io.Copy 到 md5.New，恒不整载入内存）
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, errcode.New(errcode.CodeInternal, "读取上传内容失败")
	}
	fileHash := hex.EncodeToString(h.Sum(nil))

	// (d) 判重：同分类同内容命中 → 400 源消息原文（FileApplicationService:79-80），
	// 此刻存储与 DB 零副作用（prohibition：判重命中前不得 Put）
	dup, err := s.repo.FindByCategoryAndHash(ctx, categoryID, fileHash)
	if err != nil {
		return nil, err
	}
	if dup != nil {
		return nil, errcode.New(errcode.CodeInvalidParam,
			fmt.Sprintf("文件已存在：%s（内容 MD5 重复，同一分类下不可重复上传）", fh.Filename))
	}

	// (e) Seek(0) 重放流（判重已过，流需从头供 Put 读取）
	seeker, ok := f.(io.Seeker)
	if !ok {
		return nil, errcode.New(errcode.CodeInternal, "上传流不支持重放")
	}
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		return nil, errcode.New(errcode.CodeInternal, "重放上传流失败")
	}

	// (f) 对象键：uuid + "_" + 原名（源 filePath 形态对齐；uuid 前缀天然去重防
	// 同名互覆，LocalStorage resolveKey 对键做路径逃逸防御——T-4-02）
	key := uuid.NewString() + "_" + fh.Filename
	contentType := fh.Header.Get("Content-Type")

	row := &po.FileInfoPO{
		CategoryID:   categoryID,
		CategoryName: categoryName,
		FileName:     &fh.Filename,
		FilePath:     &key,
		FileType:     &contentType,
		FileSize:     &fh.Size,
		FileHash:     &fileHash,
	}

	// (g) 事务内 Create + Put（源 save DB → 上传存储同序且事务含存储调用——Put
	// 失败返回 error 整个事务回滚，sys_file 零残留）。tx 上重建 repo（NewFileInfoRepo
	// 接受 tx）保持 ctx 透传的审计钩子语义。
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repository.NewFileInfoRepo(tx).Create(ctx, row); err != nil {
			return err
		}
		return s.storage.Put(ctx, key, f, fh.Size, contentType)
	})
	if txErr != nil {
		return nil, txErr
	}
	return dto.ToFileInfoResp(row), nil
}

// DownloadResult 下载链载体：DB 行（响应头与契约回显）+ 内容流 + 字节数 + MIME。
type DownloadResult struct {
	Row         *po.FileInfoPO
	Reader      io.ReadCloser
	Size        int64
	ContentType string
}

// Download 下载链（FILE-02 流式，全链不整载）：FindByID（缺失 → 404「文件不存在:
// {id}」源消息原文）→ FilePath 空值防御（「文件路径为空，文件ID: {id}」源消息
// 原文）→ storage.GetStream 返回 reader/size/contentType。调用方负责 Close。
func (s *FileService) Download(ctx context.Context, id int64) (*DownloadResult, error) {
	row, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("文件不存在: %d", id))
		}
		return nil, err
	}
	if row.FilePath == nil || *row.FilePath == "" {
		return nil, errcode.New(errcode.CodeInvalidParam, fmt.Sprintf("文件路径为空，文件ID: %d", id))
	}
	contentType := "application/octet-stream"
	if row.FileType != nil && *row.FileType != "" {
		contentType = *row.FileType
	}
	reader, size, err := s.storage.GetStream(ctx, *row.FilePath)
	if err != nil {
		return nil, err
	}
	return &DownloadResult{Row: row, Reader: reader, Size: size, ContentType: contentType}, nil
}

// Delete 删除链（FILE-03 宽容语义，源 FileApplicationService:112-122 逐字）：
// FindByID → storage.Delete 失败仅 slog.Warn（消息含 filePath，源日志原文）继续 →
// repo.Delete——存储失败不阻断 DB 清理。文件不存在 → 404 源消息原文。
func (s *FileService) Delete(ctx context.Context, id int64) error {
	row, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return errcode.New(errcode.CodeNotFound, fmt.Sprintf("文件不存在: %d", id))
		}
		return err
	}
	if row.FilePath != nil && *row.FilePath != "" {
		if err := s.storage.Delete(ctx, *row.FilePath); err != nil {
			// 宽容语义：仅告警留痕（源 warn 原文），继续删 DB 记录
			slog.Warn(fmt.Sprintf("S3对象删除失败，继续删除数据库记录: %s", *row.FilePath), "error", err.Error())
		}
	}
	return s.repo.Delete(ctx, id)
}

// List 分页列表（源 /file list 语义直译）：categoryID 非 nil 可选过滤。
func (s *FileService) List(ctx context.Context, page, size int, categoryID *int64) (*paging.PageData[dto.FileInfoResp], error) {
	items, total, err := s.repo.List(ctx, page, size, categoryID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.FileInfoResp, 0, len(items))
	for i := range items {
		out = append(out, *dto.ToFileInfoResp(&items[i]))
	}
	return &paging.PageData[dto.FileInfoResp]{Items: out, Total: total, Page: page, Size: size}, nil
}
