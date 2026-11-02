// Excel 三键 upsert 导入（源 FaultInfoApplicationService.importExcel
// 62-90 蓝本）：8 列契约严格解析 + 行数/表头双前置防线 + 整批单事务
// （源 @Transactional 语义）+ ImportResult 契约（FEATURES.md:205）。
//
// 8 列契约（源 FaultInfo @ExcelProperty 声明序，锁定照抄不重排、
// 禁容错重排——列序表 04-RESEARCH Implementation Guidance）：
//
//	列 1 故障代码(code) / 列 2 故障变量(name，三键，空→skip) / 列 3 触发位
//	(bit_index，空→-1 源字段初始值) / 列 4 报警类型(type，中文/int 双形态) /
//	列 5 故障描述(detail) / 列 6 故障原因(reason) / 列 7 故障位置(location，
//	三键，空→NULL) / 列 8 解决方法(fix)
//
// dict_data_id 无 @ExcelProperty：不进 Excel 不进导入（设计锁定）。
package service

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// maxImportRows 导入行数上限（含表头计，内存防线）：流式迭代读取
// 中逐行计数，超限在读取流阶段立即拒绝、不再继续读——不存在 GetRows 全量载入
// 步骤，20MB xlsx zip 炸弹（高压缩比工作表）无「解压后数百 MB～GB 全量缓冲」
// 放大面。工控机内存推导：5000 行 × ~2KB/行（8 列 × 平均 250 字符）≈ 10MB
// 解析上界，叠加 BodyLimitWithMB(20) 体积封顶与 MaxMultipartMemory 8MB 落盘
// 阈值，三维封顶（DoS 缓解）。
const maxImportRows = 5000

// faultImportHeaders 契约表头文案（列序即 8 列契约，逐一全等比较——禁容错）。
var faultImportHeaders = []string{"故障代码", "故障变量", "触发位", "报警类型", "故障描述", "故障原因", "故障位置", "解决方法"}

// strPtr 空单元格 → nil 存 NULL（源 EasyExcel/Fesod 注解读 String 字段空单元
// 格得 null 的语义）；非空 → 指针。
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ImportExcel Excel 导入全链。防线顺序即解析顺序（设计纪律：
// 行数上限与表头校验必须在解析用户数据前生效）：
//
//	(a) OpenReader + Rows("Sheet1") 流式行迭代器（源 ExcelHelper 默认 sheet 名；
//	    替代 GetRows 全量载入——行数防线在读取流阶段即生效，超限文件读到
//	    上限立即中断读取，无全量缓冲面）
//	(b) 表头严格匹配（首行）：前 8 列逐一全等 → 不符 400 整批拒绝
//	(c) 行数上限（含表头计）：流式逐行计数，超限在读取阶段立即 400 整批拒绝
//	(d) 逐行解析：len(row) > idx 边界检查（流式 Columns 同样跳过尾部空单元格、
//	    行长度不一致是常态）；name 空 → skip 不计数（源 name==null skip）；
//	    任一可解析列失败（触发位非数字等）→ 行错误上抛
//	(e) 整批单事务：tx 变体 repo FindByThreeKeys 命中 → Patch 复用原行 id →
//	    updateCount++；未命中 → Create → insertCount++；任一行 error 上抛即
//	    全批无痕回滚（源 @Transactional 语义）
//	(f) 返回 ImportResult{insertCount, updateCount}
func (s *FaultService) ImportExcel(ctx context.Context, r io.Reader) (*dto.ImportResult, error) {
	// (a) 打开工作簿并取 Sheet1 流式行迭代器（行数防线前移到读取流阶段）
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, errcode.New(errcode.CodeInvalidParam, "Excel 文件无法解析")
	}
	defer f.Close()
	iter, err := f.Rows("Sheet1")
	if err != nil {
		return nil, errcode.New(errcode.CodeInvalidParam, "Excel 缺少 Sheet1 工作表")
	}
	defer iter.Close()

	// (b)(c)(d) 表头校验 → 流式逐行计数与解析（prohibition：行数上限与表头校验
	// 先于用户数据解析生效；超限读到即断，不继续消费迭代器）
	parsed := make([]*po.FaultInfoPO, 0, maxImportRows-1)
	rowCount := 0 // 物理行计数（含表头与 skip 行，与原 len(rows) 语义一致）
	for iter.Next() {
		row, colErr := iter.Columns()
		if colErr != nil {
			return nil, errcode.New(errcode.CodeInvalidParam, "Excel 文件无法解析")
		}
		rowCount++
		if rowCount == 1 {
			// (b) 表头严格匹配：前 8 列逐一与契约文案全等（列序禁重排禁容错，
			// 行尾空单元格跳过，不足 8 列的表头行按缺列处理即不符）
			for i, want := range faultImportHeaders {
				got := ""
				if len(row) > i {
					got = row[i]
				}
				if got != want {
					return nil, errcode.New(errcode.CodeInvalidParam, "Excel 表头与模板不符")
				}
			}
			continue
		}
		// (c) 行数上限（含表头计）：超限文件在读取流阶段拒绝——不进逐行解析、
		// 不再继续读后续行（内存防线前置）
		if rowCount > maxImportRows {
			return nil, errcode.New(errcode.CodeInvalidParam, fmt.Sprintf("导入数据超过上限 %d 行", maxImportRows))
		}
		// cell 带边界取值：流式 Columns 尾部空单元格跳过，行长度不一致是常态——
		// 越界一律取空串（04-RESEARCH 重点 6）
		cell := func(idx int) string {
			if len(row) > idx {
				return row[idx]
			}
			return ""
		}
		name := cell(1)
		if name == "" {
			// 源 name==null skip 语义：空故障变量行跳过不计数
			continue
		}
		// 触发位：空单元格 → -1（源字段初始值）；非空必须可解析为整数
		bitIndex := -1
		if raw := cell(2); raw != "" {
			v, atoiErr := strconv.Atoi(raw)
			if atoiErr != nil {
				return nil, errcode.New(errcode.CodeInvalidParam,
					fmt.Sprintf("第 %d 行触发位必须为整数: %s", rowCount, raw))
			}
			bitIndex = v
		}
		// 报警类型：中文/int 双形态解析，未知文案兜底 3(UNKNOWN) 不报错
		typeVal, _ := types.ParseFaultType(cell(3))
		parsed = append(parsed, &po.FaultInfoPO{
			Code:     strPtr(cell(0)),
			Name:     &name,
			BitIndex: &bitIndex,
			Type:     &typeVal,
			Detail:   strPtr(cell(4)),
			Reason:   strPtr(cell(5)),
			// location 空单元格 → nil 存 NULL（三键判重 eq 失效是源已接受行为，
			// Q2 照源裁定——见 FaultInfoRepo.FindByThreeKeys 注释）
			Location: strPtr(cell(6)),
			Fix:      strPtr(cell(7)),
		})
	}
	if err := iter.Error(); err != nil {
		return nil, errcode.New(errcode.CodeInvalidParam, "Excel 文件无法解析")
	}
	if rowCount == 0 {
		// 空 sheet（无任何行）：无表头可校验，与原 GetRows 空结果同译「表头不符」
		return nil, errcode.New(errcode.CodeInvalidParam, "Excel 表头与模板不符")
	}

	// (e) 整批单事务（SkipDefaultTransaction=true 下唯一事务边界，照 RoleService
	// 持 db 显式 Transaction 形态）：任一行 error 上抛即全批无痕回滚
	result := &dto.ImportResult{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := repository.NewFaultInfoRepo(tx)
		for _, m := range parsed {
			hit, findErr := repo.FindByThreeKeys(ctx, *m.Name, m.BitIndex, m.Location)
			if findErr != nil {
				return findErr
			}
			if hit != nil {
				// 三键全匹配 → UPDATE 复用原行 id；白名单仅 Excel 覆盖的 7 个业务
				// 列（name 为三键组成部分值相等无需写；dict_data_id 不进导入），
				// 不动 create_by 审计列——保留首建人（Patch 只显式并入 update_*
				// 列，create_* 天然不动）
				cols := map[string]any{
					"code":      m.Code,
					"bit_index": m.BitIndex,
					"type":      m.Type,
					"detail":    m.Detail,
					"reason":    m.Reason,
					"location":  m.Location,
					"fix":       m.Fix,
				}
				if patchErr := repo.Patch(ctx, hit.ID, cols); patchErr != nil {
					return patchErr
				}
				result.UpdateCount++
			} else {
				if createErr := repo.Create(ctx, m); createErr != nil {
					return createErr
				}
				result.InsertCount++
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// (f) ImportResult 契约（JSON 键 insertCount/updateCount）
	return result, nil
}
