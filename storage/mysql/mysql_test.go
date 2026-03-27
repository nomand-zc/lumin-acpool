package mysql

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// newTestStore 创建用于测试的 Store 和 sqlmock.Sqlmock。
func newTestStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	client := WrapSQLDB(db)
	store := &Store{
		client:            client,
		accountConverter:  NewConditionConverter(accountFieldMapping, nil),
		providerConverter: NewConditionConverter(providerFieldMapping, map[string]bool{"supported_models": true}),
	}
	t.Cleanup(func() { db.Close() })
	return store, mock
}

// ---------- WrapSQLDB / Client 基本测试 ----------

func TestWrapSQLDB_Exec(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec("DELETE FROM accounts").
		WithArgs("acc-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	client := WrapSQLDB(db)
	result, err := client.Exec(context.Background(), "DELETE FROM accounts WHERE id=?", "acc-1")
	if err != nil {
		t.Fatalf("Exec error: %v", err)
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		t.Errorf("RowsAffected = %d, want 1", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestWrapSQLDB_Query(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id"}).
		AddRow("acc-1").
		AddRow("acc-2")
	mock.ExpectQuery("SELECT id FROM accounts").WillReturnRows(rows)

	client := WrapSQLDB(db)
	var ids []string
	err = client.Query(context.Background(), func(r *sql.Rows) error {
		var id string
		if err := r.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	}, "SELECT id FROM accounts")
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("got %d rows, want 2", len(ids))
	}
}

func TestWrapSQLDB_Query_ErrBreak(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id"}).
		AddRow("acc-1").
		AddRow("acc-2").
		AddRow("acc-3")
	mock.ExpectQuery("SELECT id FROM accounts").WillReturnRows(rows)

	client := WrapSQLDB(db)
	var ids []string
	err = client.Query(context.Background(), func(r *sql.Rows) error {
		var id string
		if err := r.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
		return ErrBreak // 提前中止
	}, "SELECT id FROM accounts")
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 row scanned before break, got %d", len(ids))
	}
}

func TestWrapSQLDB_QueryRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"count"}).AddRow(42)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(rows)

	client := WrapSQLDB(db)
	var count int
	err = client.QueryRow(context.Background(), []any{&count}, "SELECT COUNT(*) FROM accounts")
	if err != nil {
		t.Fatalf("QueryRow error: %v", err)
	}
	if count != 42 {
		t.Errorf("count = %d, want 42", count)
	}
}

func TestWrapSQLDB_QueryRow_NoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id"})
	mock.ExpectQuery("SELECT id").WillReturnRows(rows)

	client := WrapSQLDB(db)
	var id string
	err = client.QueryRow(context.Background(), []any{&id}, "SELECT id FROM accounts WHERE id=?", "nonexistent")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got: %v", err)
	}
}

func TestWrapSQLDB_Transaction_Commit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	client := WrapSQLDB(db)
	err = client.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), "INSERT INTO accounts VALUES (?)", "acc-1")
		return err
	})
	if err != nil {
		t.Fatalf("Transaction error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestWrapSQLDB_Transaction_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO").WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	client := WrapSQLDB(db)
	err = client.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), "INSERT INTO accounts VALUES (?)", "acc-1")
		return err
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
