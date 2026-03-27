package mysql

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/credentials"
)

// mockCredential 实现 credentials.Credential 接口，用于测试。
type mockCredential struct {
	token string
}

func (m *mockCredential) Clone() credentials.Credential              { return &mockCredential{token: m.token} }
func (m *mockCredential) Validate() error                            { return nil }
func (m *mockCredential) GetAccessToken() string                     { return m.token }
func (m *mockCredential) GetRefreshToken() string                    { return "" }
func (m *mockCredential) GetExpiresAt() *time.Time                   { return nil }
func (m *mockCredential) IsExpired() bool                            { return false }
func (m *mockCredential) GetUserInfo() (credentials.UserInfo, error) { return credentials.UserInfo{}, nil }
func (m *mockCredential) ToMap() map[string]any                      { return map[string]any{"token": m.token} }

const testProviderTypeMysql = "test-mysql"

func init() {
	credentials.Register(testProviderTypeMysql, func(data []byte) credentials.Credential {
		return &mockCredential{token: "test-token"}
	})
}

func accountColumns() []string {
	return []string{
		"id", "provider_type", "provider_name", "credential", "status", "priority",
		"tags", "metadata", "usage_rules", "cooldown_until", "circuit_open_until",
		"created_at", "updated_at", "version",
	}
}

// ---------- GetAccount ----------

func TestGetAccount_Success(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	credJSON := `{"token":"test-token"}`
	rows := sqlmock.NewRows(accountColumns()).
		AddRow("acc-1", testProviderTypeMysql, "test-provider", []byte(credJSON), 1, 10,
			nil, nil, nil, nil, nil,
			now, now, 1)
	mock.ExpectQuery("SELECT id, provider_type").
		WithArgs("acc-1").
		WillReturnRows(rows)

	acct, err := store.GetAccount(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct.ID != "acc-1" {
		t.Errorf("ID = %q, want acc-1", acct.ID)
	}
	if acct.ProviderType != testProviderTypeMysql {
		t.Errorf("ProviderType = %q", acct.ProviderType)
	}
	if acct.Status != account.StatusAvailable {
		t.Errorf("Status = %v, want StatusAvailable", acct.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestGetAccount_WithNullableFields(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	credJSON := `{"token":"test-token"}`
	tagsJSON := `{"env":"test"}`
	metaJSON := `{"key":"value"}`
	rows := sqlmock.NewRows(accountColumns()).
		AddRow("acc-2", testProviderTypeMysql, "test-provider", []byte(credJSON), 1, 5,
			tagsJSON, metaJSON, nil, nil, nil,
			now, now, 2)
	mock.ExpectQuery("SELECT id, provider_type").
		WithArgs("acc-2").
		WillReturnRows(rows)

	acct, err := store.GetAccount(context.Background(), "acc-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct.Tags == nil {
		t.Error("expected non-nil Tags")
	}
	if acct.Metadata == nil {
		t.Error("expected non-nil Metadata")
	}
}

func TestGetAccount_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows(accountColumns())
	mock.ExpectQuery("SELECT id, provider_type").
		WithArgs("nonexistent").
		WillReturnRows(rows)

	_, err := store.GetAccount(context.Background(), "nonexistent")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetAccount_ErrNoRows(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT id, provider_type").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err := store.GetAccount(context.Background(), "missing")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetAccount_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT id, provider_type").
		WithArgs("acc-1").
		WillReturnError(errors.New("connection error"))

	_, err := store.GetAccount(context.Background(), "acc-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- UpdateAccount (simple, no status field) ----------

func TestUpdateAccount_Priority_Success(t *testing.T) {
	store, mock := newTestStore(t)

	// 只更新 priority（不含 status），走简单路径
	mock.ExpectExec("UPDATE accounts SET").
		WillReturnResult(sqlmock.NewResult(0, 1))

	acct := &account.Account{
		ID:           "acc-1",
		ProviderType: testProviderTypeMysql,
		ProviderName: "test-provider",
		Priority:     20,
		Version:      1,
	}
	err := store.UpdateAccount(context.Background(), acct, storage.UpdateFieldPriority)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestUpdateAccount_VersionConflict(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE accounts SET").
		WillReturnResult(sqlmock.NewResult(0, 0))

	acct := &account.Account{
		ID:       "acc-1",
		Priority: 20,
		Version:  99, // 旧版本
	}
	err := store.UpdateAccount(context.Background(), acct, storage.UpdateFieldPriority)
	if !errors.Is(err, storage.ErrVersionConflict) {
		t.Errorf("expected ErrVersionConflict, got: %v", err)
	}
}

func TestUpdateAccount_NoFields(t *testing.T) {
	store, _ := newTestStore(t)

	acct := &account.Account{ID: "acc-1", Version: 1}
	err := store.UpdateAccount(context.Background(), acct, 0)
	if err == nil {
		t.Fatal("expected error for no fields, got nil")
	}
}

func TestUpdateAccount_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE accounts SET").
		WillReturnError(errors.New("db error"))

	acct := &account.Account{
		ID:       "acc-1",
		Priority: 5,
		Version:  1,
	}
	err := store.UpdateAccount(context.Background(), acct, storage.UpdateFieldPriority)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- RemoveAccounts ----------

func TestRemoveAccounts_Success(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("DELETE FROM accounts WHERE").
		WillReturnResult(sqlmock.NewResult(0, 3))

	err := store.RemoveAccounts(context.Background(), &storage.SearchFilter{
		ProviderType: "kiro",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestRemoveAccounts_NilFilter(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("DELETE FROM accounts WHERE").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := store.RemoveAccounts(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveAccounts_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("DELETE FROM accounts WHERE").
		WillReturnError(errors.New("db error"))

	if err := store.RemoveAccounts(context.Background(), nil); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- CountAccounts ----------

func TestCountAccounts_Success(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(7)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM accounts WHERE").
		WillReturnRows(rows)

	count, err := store.CountAccounts(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 7 {
		t.Errorf("count = %d, want 7", count)
	}
}

func TestCountAccounts_WithFilter(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(3)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM accounts WHERE").
		WillReturnRows(rows)

	count, err := store.CountAccounts(context.Background(), &storage.SearchFilter{
		ProviderType: "kiro",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestCountAccounts_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM accounts WHERE").
		WillReturnError(errors.New("db error"))

	_, err := store.CountAccounts(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- SearchAccounts ----------

func TestSearchAccounts_Success(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	credJSON := `{"token":"test-token"}`
	rows := sqlmock.NewRows(accountColumns()).
		AddRow("acc-1", testProviderTypeMysql, "test-provider", []byte(credJSON), 1, 10,
			nil, nil, nil, nil, nil, now, now, 1).
		AddRow("acc-2", testProviderTypeMysql, "test-provider", []byte(credJSON), 1, 5,
			nil, nil, nil, nil, nil, now, now, 1)
	mock.ExpectQuery("SELECT id, provider_type").
		WillReturnRows(rows)

	accounts, err := store.SearchAccounts(context.Background(), &storage.SearchFilter{
		ProviderType: testProviderTypeMysql,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 2 {
		t.Errorf("got %d accounts, want 2", len(accounts))
	}
}

func TestSearchAccounts_NilFilter(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows(accountColumns())
	mock.ExpectQuery("SELECT id, provider_type").
		WillReturnRows(rows)

	accounts, err := store.SearchAccounts(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("expected empty, got %d", len(accounts))
	}
}

func TestSearchAccounts_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT id, provider_type").
		WillReturnError(errors.New("db error"))

	_, err := store.SearchAccounts(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- buildAccountWhereClause / buildAccountWhereArgs ----------

func TestBuildAccountWhereClause_NilFilter(t *testing.T) {
	condResult := &CondConvertResult{Cond: "1=1", Args: nil}
	clause := buildAccountWhereClause(nil, condResult)
	if clause != "1=1" {
		t.Errorf("clause = %q, want \"1=1\"", clause)
	}
}

func TestBuildAccountWhereClause_WithAllFields(t *testing.T) {
	condResult := &CondConvertResult{Cond: "1=1", Args: nil}
	filter := &storage.SearchFilter{
		ProviderType: "kiro",
		ProviderName: "kiro-a",
		Status:       1,
	}
	clause := buildAccountWhereClause(filter, condResult)
	for _, want := range []string{"provider_type=?", "provider_name=?", "status=?", "1=1"} {
		if !containsStr(clause, want) {
			t.Errorf("clause %q should contain %q", clause, want)
		}
	}
}

func TestBuildAccountWhereArgs_NilFilter(t *testing.T) {
	condResult := &CondConvertResult{Cond: "1=1", Args: []any{"extra"}}
	args := buildAccountWhereArgs(nil, condResult)
	if len(args) != 1 || args[0] != "extra" {
		t.Errorf("args = %v", args)
	}
}

func TestBuildAccountWhereArgs_WithFilter(t *testing.T) {
	condResult := &CondConvertResult{Cond: "1=1", Args: nil}
	filter := &storage.SearchFilter{
		ProviderType: "kiro",
		ProviderName: "kiro-a",
		Status:       1,
	}
	args := buildAccountWhereArgs(filter, condResult)
	if len(args) != 3 {
		t.Errorf("args len = %d, want 3", len(args))
	}
}

// ---------- buildProviderWhereClause / buildProviderWhereArgs ----------

func TestBuildProviderWhereClause_NilFilter(t *testing.T) {
	condResult := &CondConvertResult{Cond: "1=1", Args: nil}
	clause := buildProviderWhereClause(nil, condResult)
	if clause != "1=1" {
		t.Errorf("clause = %q", clause)
	}
}

func TestBuildProviderWhereClause_WithSupportedModel(t *testing.T) {
	condResult := &CondConvertResult{Cond: "1=1", Args: nil}
	filter := &storage.SearchFilter{
		ProviderType:   "gemini",
		SupportedModel: "gemini-pro",
	}
	clause := buildProviderWhereClause(filter, condResult)
	if !containsStr(clause, "JSON_CONTAINS") {
		t.Errorf("clause should contain JSON_CONTAINS, got: %q", clause)
	}
}

func TestBuildProviderWhereArgs_WithSupportedModel(t *testing.T) {
	condResult := &CondConvertResult{Cond: "1=1", Args: nil}
	filter := &storage.SearchFilter{
		ProviderType:   "gemini",
		SupportedModel: "gemini-pro",
	}
	args := buildProviderWhereArgs(filter, condResult)
	// provider_type + supported_model = 2 args
	if len(args) != 2 {
		t.Errorf("args len = %d, want 2", len(args))
	}
}

// helper
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStrHelper(s, sub))
}

func containsStrHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
