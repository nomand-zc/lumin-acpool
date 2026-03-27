package mysql

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func statsColumns() []string {
	return []string{
		"account_id", "total_calls", "success_calls", "failed_calls",
		"consecutive_failures", "last_used_at", "last_error_at", "last_error_msg",
	}
}

// ---------- GetStats ----------

func TestGetStats_Success(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	rows := sqlmock.NewRows(statsColumns()).
		AddRow("acc-1", 10, 8, 2, 0, now, now, "some error")
	mock.ExpectQuery("SELECT account_id").
		WithArgs("acc-1").
		WillReturnRows(rows)

	stats, err := store.GetStats(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.AccountID != "acc-1" {
		t.Errorf("AccountID = %q, want acc-1", stats.AccountID)
	}
	if stats.TotalCalls != 10 {
		t.Errorf("TotalCalls = %d, want 10", stats.TotalCalls)
	}
	if stats.SuccessCalls != 8 {
		t.Errorf("SuccessCalls = %d, want 8", stats.SuccessCalls)
	}
	if stats.FailedCalls != 2 {
		t.Errorf("FailedCalls = %d, want 2", stats.FailedCalls)
	}
	if stats.LastErrorMsg != "some error" {
		t.Errorf("LastErrorMsg = %q", stats.LastErrorMsg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestGetStats_NullFields(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows(statsColumns()).
		AddRow("acc-2", 5, 5, 0, 0, nil, nil, nil)
	mock.ExpectQuery("SELECT account_id").
		WithArgs("acc-2").
		WillReturnRows(rows)

	stats, err := store.GetStats(context.Background(), "acc-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.LastUsedAt != nil {
		t.Errorf("expected nil LastUsedAt")
	}
	if stats.LastErrorAt != nil {
		t.Errorf("expected nil LastErrorAt")
	}
}

func TestGetStats_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows(statsColumns())
	mock.ExpectQuery("SELECT account_id").
		WithArgs("nonexistent").
		WillReturnRows(rows)

	stats, err := store.GetStats(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 不存在时返回零值
	if stats.AccountID != "nonexistent" {
		t.Errorf("AccountID = %q, want nonexistent", stats.AccountID)
	}
	if stats.TotalCalls != 0 {
		t.Errorf("TotalCalls = %d, want 0", stats.TotalCalls)
	}
}

func TestGetStats_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT account_id").
		WithArgs("acc-1").
		WillReturnError(errors.New("db error"))

	_, err := store.GetStats(context.Background(), "acc-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- IncrSuccess ----------

func TestIncrSuccess_Success(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("INSERT INTO account_stats").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.IncrSuccess(context.Background(), "acc-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestIncrSuccess_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("INSERT INTO account_stats").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("db error"))

	if err := store.IncrSuccess(context.Background(), "acc-1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- IncrFailure ----------

func TestIncrFailure_NewRecord(t *testing.T) {
	store, mock := newTestStore(t)

	// RowsAffected=1 表示新插入，返回 consecutive=1
	mock.ExpectExec("INSERT INTO account_stats").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	count, err := store.IncrFailure(context.Background(), "acc-1", "timeout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestIncrFailure_ExistingRecord(t *testing.T) {
	store, mock := newTestStore(t)

	// RowsAffected=2 表示更新（ON DUPLICATE KEY UPDATE），LastInsertId 返回递增后的值
	mock.ExpectExec("INSERT INTO account_stats").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(3, 2))

	count, err := store.IncrFailure(context.Background(), "acc-1", "timeout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestIncrFailure_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("INSERT INTO account_stats").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("db error"))

	_, err := store.IncrFailure(context.Background(), "acc-1", "timeout")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- UpdateLastUsed ----------

func TestUpdateLastUsed_Success(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	mock.ExpectExec("INSERT INTO account_stats").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.UpdateLastUsed(context.Background(), "acc-1", now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// ---------- GetConsecutiveFailures ----------

func TestGetConsecutiveFailures_Success(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows([]string{"consecutive_failures"}).AddRow(5)
	mock.ExpectQuery("SELECT consecutive_failures").
		WithArgs("acc-1").
		WillReturnRows(rows)

	count, err := store.GetConsecutiveFailures(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}

func TestGetConsecutiveFailures_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows([]string{"consecutive_failures"})
	mock.ExpectQuery("SELECT consecutive_failures").
		WithArgs("nonexistent").
		WillReturnRows(rows)

	count, err := store.GetConsecutiveFailures(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestGetConsecutiveFailures_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT consecutive_failures").
		WithArgs("acc-1").
		WillReturnError(errors.New("db error"))

	_, err := store.GetConsecutiveFailures(context.Background(), "acc-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- ResetConsecutiveFailures ----------

func TestResetConsecutiveFailures_Success(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE account_stats").
		WithArgs("acc-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.ResetConsecutiveFailures(context.Background(), "acc-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestResetConsecutiveFailures_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE account_stats").
		WithArgs("acc-1").
		WillReturnError(errors.New("db error"))

	if err := store.ResetConsecutiveFailures(context.Background(), "acc-1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- RemoveStats ----------

func TestRemoveStats_Success(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("DELETE FROM account_stats").
		WithArgs("acc-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.RemoveStats(context.Background(), "acc-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestRemoveStats_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("DELETE FROM account_stats").
		WithArgs("acc-1").
		WillReturnError(errors.New("db error"))

	if err := store.RemoveStats(context.Background(), "acc-1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- ErrNoRows 直接路径（通过 sql.ErrNoRows）----------
func TestGetStats_ErrNoRowsDirectly(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT account_id").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	// GetStats 对 ErrNoRows 的处理是返回零值 AccountStats，非 error
	stats, err := store.GetStats(context.Background(), "missing")
	if err != nil {
		t.Fatalf("expected nil err for ErrNoRows, got: %v", err)
	}
	if stats == nil || stats.AccountID != "missing" {
		t.Errorf("unexpected stats: %+v", stats)
	}
}
