package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

func (s *Store) GetProvider(ctx context.Context, key account.ProviderKey) (*account.ProviderInfo, error) {
	var (
		info           account.ProviderInfo
		statusInt      int
		tagsJSON       sql.NullString
		modelsJSON     sql.NullString
		usageRulesJSON sql.NullString
		metadataJSON   sql.NullString
		createdAtStr   string
		updatedAtStr   string
	)

	dest := []any{
		&info.ProviderType, &info.ProviderName,
		&statusInt, &info.Priority, &info.Weight,
		&tagsJSON, &modelsJSON, &usageRulesJSON, &metadataJSON,
		&info.AccountCount, &info.AvailableAccountCount,
		&createdAtStr, &updatedAtStr,
	}

	err := s.client.QueryRow(ctx, dest, queryGetProvider, key.Type, key.Name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("sqlite store: failed to get provider: %w", err)
	}

	result, err := buildProviderInfo(&info, statusInt, tagsJSON, modelsJSON, usageRulesJSON, metadataJSON, createdAtStr, updatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("sqlite store: failed to build provider info: %w", err)
	}
	return result, nil
}

func (s *Store) SearchProviders(ctx context.Context, filter *storage.SearchFilter) ([]*account.ProviderInfo, error) {
	var extraCond *filtercond.Filter
	if filter != nil {
		extraCond = filter.ExtraCond
	}
	condResult, err := s.providerConverter.Convert(extraCond)
	if err != nil {
		return nil, fmt.Errorf("sqlite store: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`SELECT `+providerSelectColumns+` FROM providers WHERE %s`, buildProviderWhereClause(filter, condResult))
	args := buildProviderWhereArgs(filter, condResult)

	var result []*account.ProviderInfo
	err = s.client.Query(ctx, func(rows *sql.Rows) error {
		info, scanErr := scanProviderFields(rows)
		if scanErr != nil {
			return fmt.Errorf("sqlite store: failed to scan provider: %w", scanErr)
		}
		result = append(result, info)
		return nil
	}, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite store: failed to search providers: %w", err)
	}
	return result, nil
}

func buildProviderWhereClause(filter *storage.SearchFilter, condResult *CondConvertResult) string {
	parts := []string{}
	if filter != nil {
		if filter.ProviderType != "" {
			parts = append(parts, "provider_type=?")
		}
		if filter.ProviderName != "" {
			parts = append(parts, "provider_name=?")
		}
		if filter.Status != 0 {
			parts = append(parts, "status=?")
		}
		if filter.SupportedModel != "" {
			parts = append(parts, `EXISTS(SELECT 1 FROM json_each(CAST("supported_models" AS TEXT)) WHERE json_each.value = ?)`)
		}
	}
	parts = append(parts, condResult.Cond)
	return strings.Join(parts, " AND ")
}

func buildProviderWhereArgs(filter *storage.SearchFilter, condResult *CondConvertResult) []any {
	var args []any
	if filter != nil {
		if filter.ProviderType != "" {
			args = append(args, filter.ProviderType)
		}
		if filter.ProviderName != "" {
			args = append(args, filter.ProviderName)
		}
		if filter.Status != 0 {
			args = append(args, filter.Status)
		}
		if filter.SupportedModel != "" {
			args = append(args, filter.SupportedModel)
		}
	}
	args = append(args, condResult.Args...)
	return args
}

func (s *Store) AddProvider(ctx context.Context, info *account.ProviderInfo) error {
	now := time.Now()
	createdAt := info.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	tagsJSON, err := MarshalJSON(info.Tags)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to marshal tags: %w", err)
	}
	modelsJSON, err := MarshalJSON(info.SupportedModels)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to marshal supported_models: %w", err)
	}
	usageRulesJSON, err := MarshalJSON(info.UsageRules)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to marshal usage_rules: %w", err)
	}
	metadataJSON, err := MarshalJSON(info.Metadata)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to marshal metadata: %w", err)
	}

	_, err = s.client.Exec(ctx, queryInsertProvider,
		info.ProviderType, info.ProviderName,
		int(info.Status), info.Priority, info.Weight,
		tagsJSON, modelsJSON, usageRulesJSON, metadataJSON,
		info.AccountCount, info.AvailableAccountCount,
		createdAt.Format("2006-01-02 15:04:05.000"),
		now.Format("2006-01-02 15:04:05.000"),
	)
	if err != nil {
		if IsDuplicateEntry(err) {
			return storage.ErrAlreadyExists
		}
		return fmt.Errorf("sqlite store: failed to add provider: %w", err)
	}
	return nil
}

func (s *Store) UpdateProvider(ctx context.Context, info *account.ProviderInfo) error {
	tagsJSON, err := MarshalJSON(info.Tags)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to marshal tags: %w", err)
	}
	modelsJSON, err := MarshalJSON(info.SupportedModels)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to marshal supported_models: %w", err)
	}
	usageRulesJSON, err := MarshalJSON(info.UsageRules)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to marshal usage_rules: %w", err)
	}
	metadataJSON, err := MarshalJSON(info.Metadata)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to marshal metadata: %w", err)
	}

	result, err := s.client.Exec(ctx, queryUpdateProvider,
		int(info.Status), info.Priority, info.Weight,
		tagsJSON, modelsJSON, usageRulesJSON, metadataJSON,
		info.AccountCount, info.AvailableAccountCount,
		time.Now().Format("2006-01-02 15:04:05.000"),
		info.ProviderType, info.ProviderName,
	)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to update provider: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite store: failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (s *Store) RemoveProvider(ctx context.Context, key account.ProviderKey) error {
	result, err := s.client.Exec(ctx, queryDeleteProvider, key.Type, key.Name)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to remove provider: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite store: failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return storage.ErrNotFound
	}
	return nil
}
