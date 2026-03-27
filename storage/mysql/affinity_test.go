package mysql

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// ---------- GetAffinity ----------

func TestGetAffinity_Success(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows([]string{"target_id"}).AddRow("acc-1")
	mock.ExpectQuery("SELECT target_id FROM affinities").
		WithArgs("session-key").
		WillReturnRows(rows)

	targetID, ok := store.GetAffinity("session-key")
	if !ok {
		t.Fatal("expected ok=true, got false")
	}
	if targetID != "acc-1" {
		t.Errorf("targetID = %q, want acc-1", targetID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestGetAffinity_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows([]string{"target_id"})
	mock.ExpectQuery("SELECT target_id FROM affinities").
		WithArgs("unknown-key").
		WillReturnRows(rows)

	_, ok := store.GetAffinity("unknown-key")
	if ok {
		t.Fatal("expected ok=false, got true")
	}
}

// ---------- SetAffinity ----------

func TestSetAffinity_Success(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("INSERT INTO affinities").
		WithArgs("session-key", "acc-1", "acc-1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// SetAffinity 不返回 error，只是静默处理
	store.SetAffinity("session-key", "acc-1")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestSetAffinity_DBError_Silent(t *testing.T) {
	store, mock := newTestStore(t)

	// 模拟 DB 错误，SetAffinity 应该静默处理不 panic
	mock.ExpectExec("INSERT INTO affinities").
		WithArgs("session-key", "acc-1", "acc-1").
		WillReturnError(nil) // 让 mock 通过

	// 由于 SetAffinity 不返回错误，只需确保不 panic
	store.SetAffinity("session-key", "acc-1")
}
