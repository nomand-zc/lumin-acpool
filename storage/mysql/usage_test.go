package mysql

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// ---------- GetCurrentUsages ----------

func TestGetCurrentUsages_Success(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"rule_index", "source_type", "time_granularity", "window_size", "rule_total",
		"local_used", "remote_used", "remote_remain", "window_start", "window_end", "last_sync_at",
	}).AddRow(0, 1, "day", 1, 1000.0, 100.0, 200.0, 700.0, now, now, now).
		AddRow(1, 2, "hour", 1, 500.0, 50.0, 100.0, 350.0, nil, nil, now)

	mock.ExpectQuery("SELECT rule_index").
		WithArgs("acc-1").
		WillReturnRows(rows)

	usages, err := store.GetCurrentUsages(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(usages) != 2 {
		t.Errorf("got %d usages, want 2", len(usages))
	}
	if usages[0].LocalUsed != 100.0 {
		t.Errorf("LocalUsed = %f, want 100.0", usages[0].LocalUsed)
	}
	if usages[1].WindowStart != nil {
		t.Errorf("expected nil WindowStart for second usage")
	}
}

func TestGetCurrentUsages_Empty(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows([]string{
		"rule_index", "source_type", "time_granularity", "window_size", "rule_total",
		"local_used", "remote_used", "remote_remain", "window_start", "window_end", "last_sync_at",
	})
	mock.ExpectQuery("SELECT rule_index").
		WithArgs("acc-empty").
		WillReturnRows(rows)

	usages, err := store.GetCurrentUsages(context.Background(), "acc-empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(usages) != 0 {
		t.Errorf("expected empty, got %d", len(usages))
	}
}

func TestGetCurrentUsages_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT rule_index").
		WithArgs("acc-1").
		WillReturnError(errors.New("db error"))

	_, err := store.GetCurrentUsages(context.Background(), "acc-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- RemoveUsages ----------

func TestRemoveUsages_Success(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("DELETE FROM tracked_usages").
		WithArgs("acc-1").
		WillReturnResult(sqlmock.NewResult(0, 3))

	if err := store.RemoveUsages(context.Background(), "acc-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestRemoveUsages_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("DELETE FROM tracked_usages").
		WithArgs("acc-1").
		WillReturnError(errors.New("db error"))

	if err := store.RemoveUsages(context.Background(), "acc-1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- IncrLocalUsed ----------

func TestIncrLocalUsed_Success(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE tracked_usages SET local_used").
		WithArgs(10.0, "acc-1", 0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.IncrLocalUsed(context.Background(), "acc-1", 0, 10.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestIncrLocalUsed_NoRows_SilentIgnore(t *testing.T) {
	store, mock := newTestStore(t)

	// 记录不存在，RowsAffected=0，应静默忽略
	mock.ExpectExec("UPDATE tracked_usages SET local_used").
		WithArgs(10.0, "acc-new", 0).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := store.IncrLocalUsed(context.Background(), "acc-new", 0, 10.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIncrLocalUsed_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE tracked_usages SET local_used").
		WithArgs(10.0, "acc-1", 0).
		WillReturnError(errors.New("db error"))

	if err := store.IncrLocalUsed(context.Background(), "acc-1", 0, 10.0); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- CalibrateRule ----------

func TestCalibrateRule_Success(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	mock.ExpectExec("UPDATE tracked_usages SET").
		WillReturnResult(sqlmock.NewResult(0, 1))

	usage := &account.TrackedUsage{
		Rule: &usagerule.UsageRule{
			Total: 1000.0,
		},
		RemoteUsed:   300.0,
		RemoteRemain: 700.0,
		WindowStart:  &now,
		WindowEnd:    &now,
	}
	if err := store.CalibrateRule(context.Background(), "acc-1", 0, usage); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestCalibrateRule_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE tracked_usages SET").
		WillReturnError(errors.New("db error"))

	usage := &account.TrackedUsage{RemoteUsed: 100.0}
	if err := store.CalibrateRule(context.Background(), "acc-1", 0, usage); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- SaveUsages ----------

func TestSaveUsages_Success(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM tracked_usages").
		WithArgs("acc-1").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectPrepare("INSERT INTO tracked_usages")
	mock.ExpectExec("INSERT INTO tracked_usages").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	usages := []*account.TrackedUsage{
		{
			Rule: &usagerule.UsageRule{
				SourceType:      usagerule.SourceType(1),
				TimeGranularity: usagerule.TimeGranularity("day"),
				WindowSize:      1,
				Total:           1000.0,
			},
			LocalUsed:    50.0,
			RemoteUsed:   100.0,
			RemoteRemain: 850.0,
			WindowStart:  &now,
			WindowEnd:    &now,
			LastSyncAt:   now,
		},
	}
	if err := store.SaveUsages(context.Background(), "acc-1", usages); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestSaveUsages_EmptyUsages(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM tracked_usages").
		WithArgs("acc-1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := store.SaveUsages(context.Background(), "acc-1", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestSaveUsages_DeleteError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM tracked_usages").
		WithArgs("acc-1").
		WillReturnError(errors.New("delete failed"))
	mock.ExpectRollback()

	if err := store.SaveUsages(context.Background(), "acc-1", nil); err == nil {
		t.Fatal("expected error, got nil")
	}
}
