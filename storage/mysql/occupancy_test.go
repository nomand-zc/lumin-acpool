package mysql

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// ---------- IncrOccupancy ----------

func TestIncrOccupancy_NewRecord(t *testing.T) {
	store, mock := newTestStore(t)

	// RowsAffected=1 表示新插入
	mock.ExpectExec("INSERT INTO account_occupancy").
		WithArgs("acc-1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	count, err := store.IncrOccupancy(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestIncrOccupancy_ExistingRecord(t *testing.T) {
	store, mock := newTestStore(t)

	// RowsAffected=2 表示更新（ON DUPLICATE KEY），LastInsertId 返回递增后值
	mock.ExpectExec("INSERT INTO account_occupancy").
		WithArgs("acc-1").
		WillReturnResult(sqlmock.NewResult(3, 2))

	count, err := store.IncrOccupancy(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestIncrOccupancy_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("INSERT INTO account_occupancy").
		WithArgs("acc-1").
		WillReturnError(errors.New("db error"))

	_, err := store.IncrOccupancy(context.Background(), "acc-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- DecrOccupancy ----------

func TestDecrOccupancy_Success(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE account_occupancy").
		WithArgs("acc-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.DecrOccupancy(context.Background(), "acc-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestDecrOccupancy_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE account_occupancy").
		WithArgs("acc-1").
		WillReturnError(errors.New("db error"))

	if err := store.DecrOccupancy(context.Background(), "acc-1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- GetOccupancy ----------

func TestGetOccupancy_Success(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows([]string{"count"}).AddRow(int64(7))
	mock.ExpectQuery("SELECT count FROM account_occupancy").
		WithArgs("acc-1").
		WillReturnRows(rows)

	count, err := store.GetOccupancy(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 7 {
		t.Errorf("count = %d, want 7", count)
	}
}

func TestGetOccupancy_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows([]string{"count"})
	mock.ExpectQuery("SELECT count FROM account_occupancy").
		WithArgs("nonexistent").
		WillReturnRows(rows)

	count, err := store.GetOccupancy(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestGetOccupancy_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT count FROM account_occupancy").
		WithArgs("acc-1").
		WillReturnError(errors.New("db error"))

	_, err := store.GetOccupancy(context.Background(), "acc-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- GetOccupancies ----------

func TestGetOccupancies_Empty(t *testing.T) {
	store, _ := newTestStore(t)

	result, err := store.GetOccupancies(context.Background(), []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestGetOccupancies_Success(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows([]string{"account_id", "count"}).
		AddRow("acc-1", int64(3)).
		AddRow("acc-2", int64(5))
	mock.ExpectQuery("SELECT account_id, count FROM account_occupancy WHERE account_id IN").
		WillReturnRows(rows)

	result, err := store.GetOccupancies(context.Background(), []string{"acc-1", "acc-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["acc-1"] != 3 {
		t.Errorf("acc-1 count = %d, want 3", result["acc-1"])
	}
	if result["acc-2"] != 5 {
		t.Errorf("acc-2 count = %d, want 5", result["acc-2"])
	}
}

func TestGetOccupancies_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT account_id, count FROM account_occupancy WHERE account_id IN").
		WillReturnError(errors.New("db error"))

	_, err := store.GetOccupancies(context.Background(), []string{"acc-1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
