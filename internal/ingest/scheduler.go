// 包 ingest：调度每周的离线数据刷新任务，运行在服务进程内的后台协程
package ingest

import (
	"database/sql"
	"ip-api/internal/logger"
	"os"
	"strconv"
	"time"
)

// nextMondayAt：计算下一次周一指定小时的时间点（不含当前已过时的当周）
// 约束：基于传入时区 loc 与整点 hour；仅前推至未来时间
func nextMondayAt(loc *time.Location, hour int) time.Time {
	now := time.Now().In(loc)
	d := now
	for i := 0; i <= 7; i++ {
		d = now.AddDate(0, 0, i)
		if d.Weekday() == time.Monday {
			t := time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, loc)
			if t.After(now) {
				return t
			}
		}
	}
	d = now.AddDate(0, 0, 7)
	return time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, loc)
}

// StartWeeklyShanghai：在北京时间（Asia/Shanghai）每周一 3:00 启动刷新任务
// 背景：遵循上游更新节奏进行定期刷新；错误由日志记录，任务继续调度
// 约束：可使用 INGEST_HOUR 覆盖小时（整数），不支持分钟级；运行于后台协程
func StartWeeklyShanghai(db *sql.DB, srcURL string) {
	l := logger.L()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	hour := 3
	if h := os.Getenv("INGEST_HOUR"); h != "" {
		if n, err := strconv.Atoi(h); err == nil {
			hour = n
		}
	}
	next := nextMondayAt(loc, hour)
	go func() {
		for {
			time.Sleep(time.Until(next))
			l.Info("ingest_start", "next", next)
			if err := FetchAndImport(db, srcURL); err != nil {
				l.Error("ingest_error", "err", err)
			} else {
				l.Info("ingest_done")
			}
			next = next.AddDate(0, 0, 7)
		}
	}()
}
