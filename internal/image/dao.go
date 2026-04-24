package image

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// ErrNotFound 鏈壘鍒颁换鍔°€?
var ErrNotFound = errors.New("image: task not found")

// DAO image_tasks 琛ㄨ闂璞°€?
type DAO struct{ db *sqlx.DB }

// NewDAO 鏋勯€犮€?
func NewDAO(db *sqlx.DB) *DAO { return &DAO{db: db} }

// Create 鎻掑叆鏂颁换鍔°€?
func (d *DAO) Create(ctx context.Context, t *Task) error {
	res, err := d.db.ExecContext(ctx, `
INSERT INTO image_tasks
  (task_id, user_id, key_id, model_id, account_id, prompt, n, size, upscale, status,
   conversation_id, file_ids, result_urls, error, estimated_credit, credit_cost,
   created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?, NOW())`,
		t.TaskID, t.UserID, t.KeyID, t.ModelID, t.AccountID,
		t.Prompt, t.N, t.Size, ValidateUpscale(t.Upscale),
		nullEmpty(t.Status, StatusQueued),
		t.ConversationID, nullJSON(t.FileIDs), nullJSON(t.ResultURLs),
		t.Error, t.EstimatedCredit, t.CreditCost,
	)
	if err != nil {
		return fmt.Errorf("image dao create: %w", err)
	}
	id, _ := res.LastInsertId()
	t.ID = uint64(id)
	return nil
}

// MarkRunning 鏍囪涓鸿繍琛屼腑(璁板綍璧峰鏃堕棿 + account_id)銆?
func (d *DAO) MarkRunning(ctx context.Context, taskID string, accountID uint64) error {
	_, err := d.db.ExecContext(ctx, `
UPDATE image_tasks
   SET status='running', account_id=?, started_at=NOW()
 WHERE task_id=? AND status IN ('queued','dispatched')`, accountID, taskID)
	return err
}

// SetAccount 鍦?runOnce 鎷垮埌璐﹀彿 lease 鍚庣珛鍒诲啓鍏?account_id銆?
// 鐙珛鍑烘潵鏄洜涓?MarkRunning 鍙湪 status=queued/dispatched 鏃剁敓鏁?
// 鑰岃皟搴﹀畬鎴愬悗 status 宸茬粡鏄?running,闇€瑕佷竴涓箓绛夌殑灏忔柟娉曘€?
// 鍥剧墖浠ｇ悊绔偣鎸?task_id 鏌ヨ处鍙锋椂渚濊禆杩欎釜瀛楁銆?
func (d *DAO) SetAccount(ctx context.Context, taskID string, accountID uint64) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE image_tasks SET account_id = ? WHERE task_id = ?`, accountID, taskID)
	return err
}

// MarkSuccess 鏇存柊鎴愬姛鐘舵€併€?
func (d *DAO) MarkSuccess(ctx context.Context, taskID, convID string, fileIDs, resultURLs []string, creditCost int64) error {
	fidB, _ := json.Marshal(fileIDs)
	urlB, _ := json.Marshal(resultURLs)
	_, err := d.db.ExecContext(ctx, `
UPDATE image_tasks
   SET status='success',
       conversation_id=?,
       file_ids=?,
       result_urls=?,
       credit_cost=?,
       finished_at=NOW()
 WHERE task_id=?`, convID, fidB, urlB, creditCost, taskID)
	return err
}

// UpdateCost 浠呮洿鏂?credit_cost(Runner 鎴愬姛鍚庣敱缃戝叧灞傝皟鐢?銆?
func (d *DAO) UpdateCost(ctx context.Context, taskID string, cost int64) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE image_tasks SET credit_cost = ? WHERE task_id = ?`, cost, taskID)
	return err
}

// MarkFailed 鏇存柊澶辫触鐘舵€?甯﹂敊璇爜)銆?
func (d *DAO) MarkFailed(ctx context.Context, taskID, errorCode string) error {
	_, err := d.db.ExecContext(ctx, `
UPDATE image_tasks
   SET status='failed', error=?, finished_at=NOW()
 WHERE task_id=?`, truncate(errorCode, 500), taskID)
	return err
}

// Get 鏍规嵁瀵瑰 task_id 鏌ヨ銆?
func (d *DAO) Get(ctx context.Context, taskID string) (*Task, error) {
	var t Task
	err := d.db.GetContext(ctx, &t, `
SELECT id, task_id, user_id, key_id, model_id, account_id, prompt, n, size, upscale, status,
       conversation_id, file_ids, result_urls, error, estimated_credit, credit_cost,
       created_at, started_at, finished_at
  FROM image_tasks
 WHERE task_id = ?`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListByUser 鎸夌敤鎴峰垎椤点€?
func (d *DAO) ListByUser(ctx context.Context, userID uint64, limit, offset int) ([]Task, error) {
	if limit <= 0 {
		limit = 20
	}
	var out []Task
	err := d.db.SelectContext(ctx, &out, `
SELECT id, task_id, user_id, key_id, model_id, account_id, prompt, n, size, upscale, status,
       conversation_id, file_ids, result_urls, error, estimated_credit, credit_cost,
       created_at, started_at, finished_at
  FROM image_tasks
 WHERE user_id = ?
 ORDER BY id DESC
 LIMIT ? OFFSET ?`, userID, limit, offset)
	return out, err
}

// DeleteByUserTaskID 鍒犻櫎褰撳墠鐢ㄦ埛鑷繁鐨勪换鍔°€?
func (d *DAO) DeleteByUserTaskID(ctx context.Context, userID uint64, taskID string) error {
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM image_tasks WHERE user_id = ? AND task_id = ?`, userID, taskID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminTaskRow 鏄鐞嗗憳瑙嗚鐨勭敓鎴愯褰曡,JOIN 浜?users 琛ㄧ殑閭銆?
type AdminTaskRow struct {
	Task
	UserEmail string `db:"user_email" json:"user_email"`
}

// AdminTaskFilter 绠＄悊鍛樻煡璇㈣繃婊ゆ潯浠躲€?
type AdminTaskFilter struct {
	UserID  uint64
	Keyword string // 妯＄硦鍖归厤 prompt / email
	Status  string
}

// ListAdmin 鍏ㄥ眬鍒嗛〉(admin)銆?
func (d *DAO) ListAdmin(ctx context.Context, f AdminTaskFilter, limit, offset int) ([]AdminTaskRow, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	where := "1=1"
	args := []interface{}{}
	if f.UserID > 0 {
		where += " AND t.user_id = ?"
		args = append(args, f.UserID)
	}
	if f.Status != "" {
		where += " AND t.status = ?"
		args = append(args, f.Status)
	}
	if f.Keyword != "" {
		like := "%" + f.Keyword + "%"
		where += " AND (t.prompt LIKE ? OR u.email LIKE ?)"
		args = append(args, like, like)
	}

	var total int64
	countSQL := `SELECT COUNT(*) FROM image_tasks t LEFT JOIN users u ON u.id=t.user_id WHERE ` + where
	if err := d.db.GetContext(ctx, &total, countSQL, args...); err != nil {
		return nil, 0, err
	}

	listSQL := `
SELECT t.id, t.task_id, t.user_id, t.key_id, t.model_id, t.account_id,
       t.prompt, t.n, t.size, t.upscale, t.status,
       t.conversation_id, t.file_ids, t.result_urls, t.error,
       t.estimated_credit, t.credit_cost,
       t.created_at, t.started_at, t.finished_at,
       COALESCE(u.email, '') AS user_email
  FROM image_tasks t
  LEFT JOIN users u ON u.id = t.user_id
 WHERE ` + where + `
 ORDER BY t.id DESC
 LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	var out []AdminTaskRow
	err := d.db.SelectContext(ctx, &out, listSQL, args...)
	return out, total, err
}

// DecodeFileIDs 鎶?JSON 鍒楄В鍑哄瓧绗︿覆鏁扮粍銆?
func (t *Task) DecodeFileIDs() []string {
	var out []string
	if len(t.FileIDs) > 0 {
		_ = json.Unmarshal(t.FileIDs, &out)
	}
	return out
}

// DecodeResultURLs 鎶?JSON 鍒楄В鍑哄瓧绗︿覆鏁扮粍銆?
func (t *Task) DecodeResultURLs() []string {
	var out []string
	if len(t.ResultURLs) > 0 {
		_ = json.Unmarshal(t.ResultURLs, &out)
	}
	return out
}

// ---- helpers ----

func nullEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func nullJSON(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

var _ = time.Now // keep import
