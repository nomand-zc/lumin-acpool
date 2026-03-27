package mysql

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
)

// ---------- AddAccount ----------

func TestAddAccount_Success(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO accounts").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE providers SET account_count").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	acct := &account.Account{
		ID:           "acc-new",
		ProviderType: testProviderTypeMysql,
		ProviderName: "test-provider",
		Credential:   &mockCredential{token: "tok"},
		Status:       account.StatusAvailable,
		Priority:     5,
	}
	if err := store.AddAccount(context.Background(), acct); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestAddAccount_Unavailable(t *testing.T) {
	store, mock := newTestStore(t)

	// status != StatusAvailable，available_incr = 0
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO accounts").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE providers SET account_count").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	acct := &account.Account{
		ID:           "acc-disabled",
		ProviderType: testProviderTypeMysql,
		ProviderName: "test-provider",
		Credential:   &mockCredential{token: "tok"},
		Status:       account.StatusDisabled,
	}
	if err := store.AddAccount(context.Background(), acct); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddAccount_DuplicateEntry(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO accounts").
		WillReturnError(errors.New("Duplicate entry 'acc-1' for key 'PRIMARY'"))
	mock.ExpectRollback()

	acct := &account.Account{
		ID:           "acc-1",
		ProviderType: testProviderTypeMysql,
		ProviderName: "test-provider",
		Credential:   &mockCredential{token: "tok"},
	}
	err := store.AddAccount(context.Background(), acct)
	if !errors.Is(err, storage.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got: %v", err)
	}
}

func TestAddAccount_InsertError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO accounts").
		WillReturnError(errors.New("disk full"))
	mock.ExpectRollback()

	acct := &account.Account{
		ID:           "acc-1",
		ProviderType: testProviderTypeMysql,
		ProviderName: "test-provider",
		Credential:   &mockCredential{token: "tok"},
	}
	if err := store.AddAccount(context.Background(), acct); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAddAccount_ProviderUpdateError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO accounts").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE providers SET account_count").
		WillReturnError(errors.New("provider update failed"))
	mock.ExpectRollback()

	acct := &account.Account{
		ID:           "acc-1",
		ProviderType: testProviderTypeMysql,
		ProviderName: "test-provider",
		Credential:   &mockCredential{token: "tok"},
		Status:       account.StatusAvailable,
	}
	if err := store.AddAccount(context.Background(), acct); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- RemoveAccount ----------

func TestRemoveAccount_Success(t *testing.T) {
	store, mock := newTestStore(t)

	// 1. 查询账号信息
	rows := sqlmock.NewRows([]string{"provider_type", "provider_name", "status"}).
		AddRow(testProviderTypeMysql, "test-provider", 1)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT provider_type, provider_name, status FROM accounts WHERE id").
		WithArgs("acc-1").
		WillReturnRows(rows)
	// 2. 删除账号
	mock.ExpectExec("DELETE FROM accounts WHERE id").
		WithArgs("acc-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 3. 更新 Provider 计数
	mock.ExpectExec("UPDATE providers SET account_count").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := store.RemoveAccount(context.Background(), "acc-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestRemoveAccount_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows([]string{"provider_type", "provider_name", "status"})
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT provider_type, provider_name, status FROM accounts WHERE id").
		WithArgs("nonexistent").
		WillReturnRows(rows)
	mock.ExpectRollback()

	err := store.RemoveAccount(context.Background(), "nonexistent")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestRemoveAccount_QueryError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT provider_type, provider_name, status FROM accounts WHERE id").
		WithArgs("acc-1").
		WillReturnError(errors.New("db error"))
	mock.ExpectRollback()

	if err := store.RemoveAccount(context.Background(), "acc-1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRemoveAccount_DeleteError(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows([]string{"provider_type", "provider_name", "status"}).
		AddRow(testProviderTypeMysql, "test-provider", 1)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT provider_type, provider_name, status FROM accounts WHERE id").
		WithArgs("acc-1").
		WillReturnRows(rows)
	mock.ExpectExec("DELETE FROM accounts WHERE id").
		WithArgs("acc-1").
		WillReturnError(errors.New("delete error"))
	mock.ExpectRollback()

	if err := store.RemoveAccount(context.Background(), "acc-1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- UpdateAccount with status field (transaction path) ----------

func TestUpdateAccount_StatusChange_Success(t *testing.T) {
	store, mock := newTestStore(t)

	// 事务路径：先查旧状态，再执行更新
	mock.ExpectBegin()
	statusRows := sqlmock.NewRows([]string{"status"}).AddRow(2) // 旧状态 = Disabled
	mock.ExpectQuery("SELECT status FROM accounts WHERE id").
		WithArgs("acc-1").
		WillReturnRows(statusRows)
	mock.ExpectExec("UPDATE accounts SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 状态从 Disabled -> Available，delta=1，更新 provider
	mock.ExpectExec("UPDATE providers SET available_account_count").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	acct := &account.Account{
		ID:           "acc-1",
		ProviderType: testProviderTypeMysql,
		ProviderName: "test-provider",
		Status:       account.StatusAvailable, // 新状态
		Version:      1,
	}
	if err := store.UpdateAccount(context.Background(), acct, storage.UpdateFieldStatus); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestUpdateAccount_StatusChange_SameStatus(t *testing.T) {
	store, mock := newTestStore(t)

	// 状态未改变，不更新 provider
	mock.ExpectBegin()
	statusRows := sqlmock.NewRows([]string{"status"}).AddRow(1) // 旧状态 = Available
	mock.ExpectQuery("SELECT status FROM accounts WHERE id").
		WithArgs("acc-1").
		WillReturnRows(statusRows)
	mock.ExpectExec("UPDATE accounts SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 状态相同，delta=0，不更新 provider
	mock.ExpectCommit()

	acct := &account.Account{
		ID:           "acc-1",
		ProviderType: testProviderTypeMysql,
		ProviderName: "test-provider",
		Status:       account.StatusAvailable, // 新旧状态相同
		Version:      1,
	}
	if err := store.UpdateAccount(context.Background(), acct, storage.UpdateFieldStatus); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestUpdateAccount_StatusChange_VersionConflict(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	statusRows := sqlmock.NewRows([]string{"status"}).AddRow(1)
	mock.ExpectQuery("SELECT status FROM accounts WHERE id").
		WithArgs("acc-1").
		WillReturnRows(statusRows)
	// RowsAffected=0 表示版本冲突
	mock.ExpectExec("UPDATE accounts SET").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	acct := &account.Account{
		ID:      "acc-1",
		Status:  account.StatusAvailable,
		Version: 99,
	}
	err := store.UpdateAccount(context.Background(), acct, storage.UpdateFieldStatus)
	if !errors.Is(err, storage.ErrVersionConflict) {
		t.Errorf("expected ErrVersionConflict, got: %v", err)
	}
}

func TestUpdateAccount_StatusChange_GetStatusNotFound(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status FROM accounts WHERE id").
		WithArgs("missing").
		WillReturnRows(sqlmock.NewRows([]string{"status"}))
	mock.ExpectRollback()

	acct := &account.Account{
		ID:      "missing",
		Status:  account.StatusAvailable,
		Version: 1,
	}
	err := store.UpdateAccount(context.Background(), acct, storage.UpdateFieldStatus)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestUpdateAccount_StatusChange_AvailableToDisabled(t *testing.T) {
	store, mock := newTestStore(t)

	// 从 Available -> Disabled，delta=-1
	mock.ExpectBegin()
	statusRows := sqlmock.NewRows([]string{"status"}).AddRow(1) // 旧 = Available
	mock.ExpectQuery("SELECT status FROM accounts WHERE id").
		WithArgs("acc-1").
		WillReturnRows(statusRows)
	mock.ExpectExec("UPDATE accounts SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE providers SET available_account_count").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	acct := &account.Account{
		ID:           "acc-1",
		ProviderType: testProviderTypeMysql,
		ProviderName: "test-provider",
		Status:       account.StatusDisabled, // 新 = Disabled
		Version:      1,
	}
	if err := store.UpdateAccount(context.Background(), acct, storage.UpdateFieldStatus); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
