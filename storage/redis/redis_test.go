package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

// ============================
// ParseRedisDSN 测试
// ============================

func TestParseDSN_BasicFormat(t *testing.T) {
	parts, err := ParseRedisDSN("redis://localhost:6379")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parts.Addr != "localhost:6379" {
		t.Errorf("expected addr localhost:6379, got %s", parts.Addr)
	}
	if parts.Password != "" {
		t.Errorf("expected empty password, got %s", parts.Password)
	}
	if parts.DB != 0 {
		t.Errorf("expected DB 0, got %d", parts.DB)
	}
}

func TestParseDSN_WithPassword(t *testing.T) {
	parts, err := ParseRedisDSN("redis://:secret@localhost:6379")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parts.Password != "secret" {
		t.Errorf("expected password 'secret', got %s", parts.Password)
	}
	if parts.Addr != "localhost:6379" {
		t.Errorf("expected addr localhost:6379, got %s", parts.Addr)
	}
}

func TestParseDSN_WithUserAndPassword(t *testing.T) {
	parts, err := ParseRedisDSN("redis://user:mypassword@redis.example.com:6380/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parts.Password != "mypassword" {
		t.Errorf("expected password 'mypassword', got %s", parts.Password)
	}
	if parts.Addr != "redis.example.com:6380" {
		t.Errorf("expected addr redis.example.com:6380, got %s", parts.Addr)
	}
	if parts.DB != 2 {
		t.Errorf("expected DB 2, got %d", parts.DB)
	}
}

func TestParseDSN_WithDBNumber(t *testing.T) {
	parts, err := ParseRedisDSN("redis://localhost:6379/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parts.DB != 1 {
		t.Errorf("expected DB 1, got %d", parts.DB)
	}
}

func TestParseDSN_NoSchemeShorthand(t *testing.T) {
	parts, err := ParseRedisDSN("localhost:6379")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parts.Addr != "localhost:6379" {
		t.Errorf("expected addr localhost:6379, got %s", parts.Addr)
	}
}

func TestParseDSN_HostOnly_DefaultPort(t *testing.T) {
	parts, err := ParseRedisDSN("redis://localhost")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parts.Addr != "localhost:6379" {
		t.Errorf("expected addr localhost:6379, got %s", parts.Addr)
	}
}

func TestParseDSN_EmptyDSN_Error(t *testing.T) {
	_, err := ParseRedisDSN("")
	if err == nil {
		t.Error("expected error for empty DSN")
	}
}

func TestParseDSN_UnsupportedScheme_Error(t *testing.T) {
	_, err := ParseRedisDSN("mysql://localhost:3306")
	if err == nil {
		t.Error("expected error for unsupported scheme")
	}
}

func TestParseDSN_InvalidDBNumber_Error(t *testing.T) {
	_, err := ParseRedisDSN("redis://localhost:6379/abc")
	if err == nil {
		t.Error("expected error for invalid DB number")
	}
}

func TestParseDSN_Rediss_Scheme(t *testing.T) {
	parts, err := ParseRedisDSN("rediss://localhost:6380")
	if err != nil {
		t.Fatalf("unexpected error for rediss scheme: %v", err)
	}
	if parts.Addr != "localhost:6380" {
		t.Errorf("expected addr localhost:6380, got %s", parts.Addr)
	}
}

// ============================
// helper.go 测试
// ============================

func TestMarshalJSON_Nil(t *testing.T) {
	s, err := MarshalJSON(nil)
	if err != nil {
		t.Fatalf("MarshalJSON nil failed: %v", err)
	}
	if s != "" {
		t.Errorf("expected empty string for nil, got %s", s)
	}
}

func TestMarshalJSON_Value(t *testing.T) {
	s, err := MarshalJSON(map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if s == "" {
		t.Error("expected non-empty JSON string")
	}
}

func TestFormatTime_Zero(t *testing.T) {
	import_time, _ := ParseTime("")
	s := FormatTime(import_time)
	if s != "" {
		t.Errorf("expected empty string for zero time, got %s", s)
	}
}

func TestFormatTime_NonZero(t *testing.T) {
	import_time, err := ParseTime("2024-01-15T10:00:00Z")
	if err != nil {
		t.Fatalf("ParseTime failed: %v", err)
	}
	s := FormatTime(import_time)
	if s == "" {
		t.Error("expected non-empty string for non-zero time")
	}
}

func TestParseInt_Empty(t *testing.T) {
	if ParseInt("") != 0 {
		t.Error("expected 0 for empty string")
	}
}

func TestParseInt_Valid(t *testing.T) {
	if ParseInt("42") != 42 {
		t.Error("expected 42")
	}
}

func TestParseFloat64_Empty(t *testing.T) {
	if ParseFloat64("") != 0 {
		t.Error("expected 0 for empty string")
	}
}

func TestParseFloat64_Valid(t *testing.T) {
	v := ParseFloat64("3.14")
	if v < 3.13 || v > 3.15 {
		t.Errorf("expected ~3.14, got %f", v)
	}
}

func TestIsNotFound_Nil(t *testing.T) {
	if IsNotFound(nil) {
		t.Error("expected false for nil error")
	}
}

func TestParseTime_Valid(t *testing.T) {
	_, err := ParseTime("2024-01-15T10:00:00Z")
	if err != nil {
		t.Fatalf("ParseTime failed: %v", err)
	}
}

func TestParseTime_Empty(t *testing.T) {
	_, err := ParseTime("")
	if err != nil {
		t.Fatalf("ParseTime empty should return zero, got: %v", err)
	}
}

func TestParseTimePtr_Empty(t *testing.T) {
	if ParseTimePtr("") != nil {
		t.Error("expected nil for empty string")
	}
}

func TestParseTimePtr_Invalid(t *testing.T) {
	if ParseTimePtr("not-a-date") != nil {
		t.Error("expected nil for invalid date")
	}
}

func TestParseInt64_Empty(t *testing.T) {
	if ParseInt64("") != 0 {
		t.Error("expected 0 for empty string")
	}
}

func TestParseInt64_Valid(t *testing.T) {
	if ParseInt64("1234567890") != 1234567890 {
		t.Error("expected 1234567890")
	}
}

// (helper functions removed - using ParseTime directly above)

// ============================
// goRedisClient 包装方法测试
// ============================

func setupGoRedisClient(t *testing.T) Client {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return WrapGoRedis(rdb)
}

func TestGoRedisClient_SAdd_SRem_SIsMember(t *testing.T) {
	ctx := context.Background()
	c := setupGoRedisClient(t)

	if err := c.SAdd(ctx, "myset", "member1", "member2"); err != nil {
		t.Fatalf("SAdd failed: %v", err)
	}

	ok, err := c.SIsMember(ctx, "myset", "member1")
	if err != nil {
		t.Fatalf("SIsMember failed: %v", err)
	}
	if !ok {
		t.Error("expected member1 to be in set")
	}

	if err := c.SRem(ctx, "myset", "member1"); err != nil {
		t.Fatalf("SRem failed: %v", err)
	}

	ok2, _ := c.SIsMember(ctx, "myset", "member1")
	if ok2 {
		t.Error("expected member1 to be removed from set")
	}
}

func TestGoRedisClient_HDel(t *testing.T) {
	ctx := context.Background()
	c := setupGoRedisClient(t)

	if err := c.HSet(ctx, "myhash", "f1", "v1", "f2", "v2"); err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	if err := c.HDel(ctx, "myhash", "f1"); err != nil {
		t.Fatalf("HDel failed: %v", err)
	}

	val, err := c.HGet(ctx, "myhash", "f1")
	if err == nil {
		t.Errorf("expected error for deleted field, got value: %s", val)
	}
}

func TestGoRedisClient_HIncrBy(t *testing.T) {
	ctx := context.Background()
	c := setupGoRedisClient(t)

	n, err := c.HIncrBy(ctx, "counthash", "field", 5)
	if err != nil {
		t.Fatalf("HIncrBy failed: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5, got %d", n)
	}

	n2, _ := c.HIncrBy(ctx, "counthash", "field", 3)
	if n2 != 8 {
		t.Errorf("expected 8, got %d", n2)
	}
}

func TestGoRedisClient_ClientBuilderOpts(t *testing.T) {
	// 测试各 ClientBuilderOpt 函数
	o := &ClientBuilderOpts{}
	WithClientBuilderDSN("redis://localhost:6379")(o)
	if o.DSN != "redis://localhost:6379" {
		t.Errorf("WithClientBuilderDSN failed")
	}

	WithMaxRetries(3)(o)
	if o.MaxRetries != 3 {
		t.Errorf("WithMaxRetries failed")
	}

	WithPoolSize(10)(o)
	if o.PoolSize != 10 {
		t.Errorf("WithPoolSize failed")
	}

	WithMinIdleConns(2)(o)
	if o.MinIdleConns != 2 {
		t.Errorf("WithMinIdleConns failed")
	}

	dur := 5 * time.Minute
	WithConnMaxIdleTime(dur)(o)
	if o.ConnMaxIdleTime != dur {
		t.Errorf("WithConnMaxIdleTime failed")
	}

	dur2 := 10 * time.Minute
	WithConnMaxLifetime(dur2)(o)
	if o.ConnMaxLifetime != dur2 {
		t.Errorf("WithConnMaxLifetime failed")
	}

	WithKeyPrefix("test:")(o)
	if o.KeyPrefix != "test:" {
		t.Errorf("WithKeyPrefix failed")
	}

	WithExtraOptions("opt1", "opt2")(o)
	if len(o.ExtraOptions) != 2 {
		t.Errorf("WithExtraOptions failed, got %d", len(o.ExtraOptions))
	}
}

func TestDefaultClientBuilder_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)

	client, err := defaultClientBuilder(WithClientBuilderDSN("redis://" + mr.Addr()))
	if err != nil {
		t.Fatalf("defaultClientBuilder failed: %v", err)
	}
	defer client.Close()

	// 验证连接可用
	ctx := context.Background()
	if err := client.Set(ctx, "test-key", "test-value", 0); err != nil {
		t.Fatalf("Set after defaultClientBuilder failed: %v", err)
	}
}

func TestDefaultClientBuilder_NoDSN_Error(t *testing.T) {
	_, err := defaultClientBuilder()
	if err == nil {
		t.Error("expected error when no DSN provided")
	}
}
