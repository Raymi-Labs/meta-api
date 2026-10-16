package controller

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func ExportAllLogs(c *gin.Context)  { exportLogs(c, true) }
func ExportUserLogs(c *gin.Context) { exportLogs(c, false) }

// Spreadsheet apps interpret these leading characters as formulas even in
// quoted CSV fields. Numeric columns are generated separately from integers.
func logCSVText(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n\ufeff")
	if strings.ContainsAny(value[:min(1, len(value))], "\t\r\n") || strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "@") {
		return "'" + value
	}
	return value
}

func exportLogs(c *gin.Context, admin bool) {
	fail := func(status int, message string) { c.JSON(status, gin.H{"success": false, "message": message}) }
	userID := c.GetInt("id")
	if userID <= 0 {
		fail(http.StatusUnauthorized, "请先登录")
		return
	}
	start, errStart := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, errEnd := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if errStart != nil || errEnd != nil || start <= 0 || end < start || end-start > 31*86400 {
		fail(http.StatusBadRequest, "请选择有效时间范围，单次导出最多 31 天")
		return
	}
	logType, err := strconv.Atoi(c.DefaultQuery("type", "0"))
	if err != nil || logType < model.LogTypeUnknown || logType > model.LogTypeRefund {
		fail(http.StatusBadRequest, "日志类型无效")
		return
	}
	filters := model.AdminLogQueryFilters{LogType: logType, StartTimestamp: start, EndTimestamp: end,
		ModelName: c.Query("model_name"), TokenName: c.Query("token_name"), Group: c.Query("group"), RequestID: c.Query("request_id")}
	if admin {
		filters.Scope, err = parseAdminLogScope(c)
		if err != nil {
			fail(http.StatusBadRequest, err.Error())
			return
		}
		filters.Username = c.Query("username")
		if raw := c.Query("channel"); raw != "" {
			filters.Channel, err = strconv.Atoi(raw)
			if err != nil || filters.Channel < 0 {
				fail(http.StatusBadRequest, "渠道 ID 无效")
				return
			}
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	release, acquired, err := service.AcquireLogExportLock(ctx, userID)
	if err != nil {
		fail(http.StatusServiceUnavailable, "暂时无法导出，请稍后重试")
		return
	}
	if !acquired {
		fail(http.StatusTooManyRequests, "日志正在导出，请稍后重试")
		return
	}
	defer release()
	file, err := os.CreateTemp("", "newapi-log-export-*.csv")
	if err != nil {
		fail(http.StatusInternalServerError, "暂时无法导出，请稍后重试")
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	writer := csv.NewWriter(file)
	_, err = file.WriteString("\xef\xbb\xbf")
	headers := []string{"时间（UTC）", "类型", "令牌", "分组", "模型", "额度（quota）", "输入Token", "输出Token", "耗时（秒）", "流式", "请求ID", "内容"}
	if admin {
		headers = append(headers, "日志ID", "用户ID", "用户名", "渠道ID")
	}
	if err == nil {
		err = writer.Write(headers)
	}
	if err == nil {
		err = model.WalkExportLogs(ctx, filters, userID, admin, func(row *model.Log) error {
			values := []string{time.Unix(row.CreatedAt, 0).UTC().Format(time.RFC3339), strconv.Itoa(row.Type), logCSVText(row.TokenName), logCSVText(row.Group), logCSVText(row.ModelName), strconv.Itoa(row.Quota), strconv.Itoa(row.PromptTokens), strconv.Itoa(row.CompletionTokens), strconv.Itoa(row.UseTime), strconv.FormatBool(row.IsStream), logCSVText(row.RequestId), logCSVText(row.Content)}
			if admin {
				values = append(values, strconv.Itoa(row.Id), strconv.Itoa(row.UserId), logCSVText(row.Username), strconv.Itoa(row.ChannelId))
			}
			if err := writer.Write(values); err != nil {
				return err
			}
			writer.Flush()
			if err := writer.Error(); err != nil {
				return err
			}
			position, err := file.Seek(0, io.SeekCurrent)
			if err != nil {
				return err
			}
			if position > 32<<20 {
				return errors.New("导出文件超过 32 MB，请缩小筛选范围")
			}
			return nil
		})
	}
	writer.Flush()
	if err == nil {
		err = writer.Error()
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		// Only intentional validation messages are exposed; never send SQL errors.
		message := "导出失败，请缩小筛选范围后重试"
		if strings.HasPrefix(err.Error(), "导出超过") || strings.HasPrefix(err.Error(), "导出文件超过") {
			message = err.Error()
		}
		fail(http.StatusBadRequest, message)
		return
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		fail(http.StatusInternalServerError, "导出失败，请重试")
		return
	}
	info, err := file.Stat()
	if err != nil {
		fail(http.StatusInternalServerError, "导出失败，请重试")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, info.Size(), "text/csv; charset=utf-8", file, map[string]string{
		"Content-Disposition": `attachment; filename="usage-logs.csv"`,
	})
}
