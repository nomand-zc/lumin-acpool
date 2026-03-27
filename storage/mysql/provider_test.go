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
)

func providerColumns() []string {
	return []string{
		"provider_type", "provider_name", "status", "priority", "weight",
		"tags", "supported_models", "usage_rules", "metadata",
		"account_count", "available_account_count", "created_at", "updated_at",
	}
}

// ---------- GetProvider ----------

func TestGetProvider_Success(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	rows := sqlmock.NewRows(providerColumns()).
		AddRow("kiro", "kiro-team-a", 1, 10, 1.0,
			nil, nil, nil, nil,
			5, 3, now, now)
	mock.ExpectQuery("SELECT provider_type, provider_name").
		WithArgs("kiro", "kiro-team-a").
		WillReturnRows(rows)

	key := account.ProviderKey{Type: "kiro", Name: "kiro-team-a"}
	info, err := store.GetProvider(context.Background(), key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ProviderType != "kiro" {
		t.Errorf("ProviderType = %q, want kiro", info.ProviderType)
	}
	if info.ProviderName != "kiro-team-a" {
		t.Errorf("ProviderName = %q, want kiro-team-a", info.ProviderName)
	}
	if info.AccountCount != 5 {
		t.Errorf("AccountCount = %d, want 5", info.AccountCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestGetProvider_WithJSON(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	tagsJSON := `{"env":"prod"}`
	modelsJSON := `["gemini-pro","gemini-flash"]`

	rows := sqlmock.NewRows(providerColumns()).
		AddRow("gemini", "gemini-prod", 1, 5, 1.0,
			tagsJSON, modelsJSON, nil, nil,
			10, 8, now, now)
	mock.ExpectQuery("SELECT provider_type, provider_name").
		WithArgs("gemini", "gemini-prod").
		WillReturnRows(rows)

	key := account.ProviderKey{Type: "gemini", Name: "gemini-prod"}
	info, err := store.GetProvider(context.Background(), key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(info.SupportedModels) != 2 {
		t.Errorf("SupportedModels len = %d, want 2", len(info.SupportedModels))
	}
}

func TestGetProvider_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	rows := sqlmock.NewRows(providerColumns())
	mock.ExpectQuery("SELECT provider_type, provider_name").
		WithArgs("unknown", "unknown").
		WillReturnRows(rows)

	key := account.ProviderKey{Type: "unknown", Name: "unknown"}
	_, err := store.GetProvider(context.Background(), key)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetProvider_ErrNoRows(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT provider_type, provider_name").
		WithArgs("x", "y").
		WillReturnError(sql.ErrNoRows)

	key := account.ProviderKey{Type: "x", Name: "y"}
	_, err := store.GetProvider(context.Background(), key)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetProvider_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT provider_type, provider_name").
		WithArgs("kiro", "kiro-team-a").
		WillReturnError(errors.New("connection timeout"))

	key := account.ProviderKey{Type: "kiro", Name: "kiro-team-a"}
	_, err := store.GetProvider(context.Background(), key)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- AddProvider ----------

func TestAddProvider_Success(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("INSERT INTO providers").
		WillReturnResult(sqlmock.NewResult(1, 1))

	info := &account.ProviderInfo{
		ProviderType: "kiro",
		ProviderName: "kiro-team-a",
		Status:       account.ProviderStatusActive,
		Priority:     10,
		Weight:       1.0,
	}
	if err := store.AddProvider(context.Background(), info); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestAddProvider_DuplicateEntry(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("INSERT INTO providers").
		WillReturnError(errors.New("Duplicate entry 'kiro/kiro-team-a' for key 'PRIMARY'"))

	info := &account.ProviderInfo{
		ProviderType: "kiro",
		ProviderName: "kiro-team-a",
	}
	err := store.AddProvider(context.Background(), info)
	if !errors.Is(err, storage.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got: %v", err)
	}
}

func TestAddProvider_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("INSERT INTO providers").
		WillReturnError(errors.New("disk full"))

	info := &account.ProviderInfo{
		ProviderType: "kiro",
		ProviderName: "kiro-team-a",
	}
	if err := store.AddProvider(context.Background(), info); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- UpdateProvider ----------

func TestUpdateProvider_Success(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE providers SET").
		WillReturnResult(sqlmock.NewResult(0, 1))

	info := &account.ProviderInfo{
		ProviderType: "kiro",
		ProviderName: "kiro-team-a",
		Status:       account.ProviderStatusActive,
		Priority:     5,
	}
	if err := store.UpdateProvider(context.Background(), info); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestUpdateProvider_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE providers SET").
		WillReturnResult(sqlmock.NewResult(0, 0))

	info := &account.ProviderInfo{
		ProviderType: "unknown",
		ProviderName: "unknown",
	}
	err := store.UpdateProvider(context.Background(), info)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestUpdateProvider_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE providers SET").
		WillReturnError(errors.New("connection error"))

	info := &account.ProviderInfo{
		ProviderType: "kiro",
		ProviderName: "kiro-team-a",
	}
	if err := store.UpdateProvider(context.Background(), info); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- RemoveProvider ----------

func TestRemoveProvider_Success(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("DELETE FROM providers").
		WithArgs("kiro", "kiro-team-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	key := account.ProviderKey{Type: "kiro", Name: "kiro-team-a"}
	if err := store.RemoveProvider(context.Background(), key); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestRemoveProvider_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("DELETE FROM providers").
		WithArgs("x", "y").
		WillReturnResult(sqlmock.NewResult(0, 0))

	key := account.ProviderKey{Type: "x", Name: "y"}
	err := store.RemoveProvider(context.Background(), key)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestRemoveProvider_DBError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("DELETE FROM providers").
		WithArgs("kiro", "kiro-team-a").
		WillReturnError(errors.New("db error"))

	key := account.ProviderKey{Type: "kiro", Name: "kiro-team-a"}
	if err := store.RemoveProvider(context.Background(), key); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- SearchProviders ----------

func TestSearchProviders_Success(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	rows := sqlmock.NewRows(providerColumns()).
		AddRow("kiro", "kiro-a", 1, 10, 1.0, nil, nil, nil, nil, 5, 3, now, now).
		AddRow("kiro", "kiro-b", 1, 5, 1.0, nil, nil, nil, nil, 3, 2, now, now)
	mock.ExpectQuery("SELECT provider_type").
		WillReturnRows(rows)

	providers, err := store.SearchProviders(context.Background(), &storage.SearchFilter{
		ProviderType: "kiro",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(providers) != 2 {
		t.Errorf("got %d providers, want 2", len(providers))
	}
}

func TestSearchProviders_NilFilter(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	rows := sqlmock.NewRows(providerColumns()).
		AddRow("kiro", "kiro-a", 1, 10, 1.0, nil, nil, nil, nil, 5, 3, now, now)
	mock.ExpectQuery("SELECT provider_type").
		WillReturnRows(rows)

	providers, err := store.SearchProviders(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(providers) != 1 {
		t.Errorf("got %d providers, want 1", len(providers))
	}
}
