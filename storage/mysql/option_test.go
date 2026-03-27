package mysql

import (
	"testing"
	"time"
)

// ---------- DefaultOptions ----------

func TestDefaultOptions(t *testing.T) {
	o := DefaultOptions()
	if o == nil {
		t.Fatal("expected non-nil Options")
	}
	if o.DSN != "" || o.InstanceName != "" || o.SkipInitDB {
		t.Errorf("unexpected defaults: %+v", o)
	}
}

// ---------- With* option funcs ----------

func TestWithInstanceName(t *testing.T) {
	o := DefaultOptions()
	WithInstanceName("my-instance")(o)
	if o.InstanceName != "my-instance" {
		t.Errorf("InstanceName = %q, want my-instance", o.InstanceName)
	}
}

func TestWithDSN(t *testing.T) {
	o := DefaultOptions()
	dsn := "user:pass@tcp(localhost:3306)/db"
	WithDSN(dsn)(o)
	if o.DSN != dsn {
		t.Errorf("DSN = %q, want %q", o.DSN, dsn)
	}
}

func TestWithSkipInitDB(t *testing.T) {
	o := DefaultOptions()
	WithSkipInitDB(true)(o)
	if !o.SkipInitDB {
		t.Error("expected SkipInitDB=true")
	}
}

func TestWithStoreExtraOptions(t *testing.T) {
	o := DefaultOptions()
	WithStoreExtraOptions("opt1", 42)(o)
	if len(o.ExtraOptions) != 2 {
		t.Errorf("ExtraOptions len = %d, want 2", len(o.ExtraOptions))
	}
}

// ---------- ClientBuilderOpt funcs ----------

func TestWithClientBuilderDSN(t *testing.T) {
	o := &ClientBuilderOpts{}
	WithClientBuilderDSN("mydsn")(o)
	if o.DSN != "mydsn" {
		t.Errorf("DSN = %q, want mydsn", o.DSN)
	}
}

func TestWithMaxOpenConns(t *testing.T) {
	o := &ClientBuilderOpts{}
	WithMaxOpenConns(10)(o)
	if o.MaxOpenConns != 10 {
		t.Errorf("MaxOpenConns = %d, want 10", o.MaxOpenConns)
	}
}

func TestWithMaxIdleConns(t *testing.T) {
	o := &ClientBuilderOpts{}
	WithMaxIdleConns(5)(o)
	if o.MaxIdleConns != 5 {
		t.Errorf("MaxIdleConns = %d, want 5", o.MaxIdleConns)
	}
}

func TestWithConnMaxLifetime(t *testing.T) {
	o := &ClientBuilderOpts{}
	d := 10 * time.Minute
	WithConnMaxLifetime(d)(o)
	if o.ConnMaxLifetime != d {
		t.Errorf("ConnMaxLifetime = %v, want %v", o.ConnMaxLifetime, d)
	}
}

func TestWithConnMaxIdleTime(t *testing.T) {
	o := &ClientBuilderOpts{}
	d := 5 * time.Minute
	WithConnMaxIdleTime(d)(o)
	if o.ConnMaxIdleTime != d {
		t.Errorf("ConnMaxIdleTime = %v, want %v", o.ConnMaxIdleTime, d)
	}
}

func TestWithExtraOptions(t *testing.T) {
	o := &ClientBuilderOpts{}
	WithExtraOptions("a", "b")(o)
	if len(o.ExtraOptions) != 2 {
		t.Errorf("ExtraOptions len = %d, want 2", len(o.ExtraOptions))
	}
}

// ---------- RegisterInstance / GetInstance ----------

func TestRegisterAndGetInstance(t *testing.T) {
	name := "test-instance-unit"
	RegisterInstance(name, WithClientBuilderDSN("testdsn"))

	opts, ok := GetInstance(name)
	if !ok {
		t.Fatal("expected ok=true, got false")
	}
	if len(opts) != 1 {
		t.Errorf("opts len = %d, want 1", len(opts))
	}
}

func TestGetInstance_NotFound(t *testing.T) {
	_, ok := GetInstance("non-existent-instance-xyz")
	if ok {
		t.Error("expected ok=false for non-existent instance")
	}
}

// ---------- SetClientBuilder / GetClientBuilder ----------

func TestSetAndGetClientBuilder(t *testing.T) {
	original := GetClientBuilder()

	// 设置自定义 builder
	custom := func(opts ...ClientBuilderOpt) (Client, error) {
		return nil, nil
	}
	SetClientBuilder(custom)

	got := GetClientBuilder()
	if got == nil {
		t.Error("expected non-nil builder after SetClientBuilder")
	}

	// 恢复原始 builder
	SetClientBuilder(original)
}

// ---------- buildClient errors ----------

func TestBuildClient_NoInstanceNoDSN(t *testing.T) {
	o := DefaultOptions()
	_, err := buildClient(o)
	if err == nil {
		t.Fatal("expected error when neither InstanceName nor DSN is set")
	}
}

func TestBuildClient_InstanceNotFound(t *testing.T) {
	o := DefaultOptions()
	o.InstanceName = "definitely-not-registered-xyz"
	_, err := buildClient(o)
	if err == nil {
		t.Fatal("expected error for non-existent instance")
	}
}

func TestBuildClient_WithDSN_PingFail(t *testing.T) {
	// 提供无法连接的 DSN，应返回 ping 失败错误
	o := DefaultOptions()
	o.DSN = "user:pass@tcp(127.0.0.1:9999)/nonexistentdb?parseTime=true"
	_, err := buildClient(o)
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}
}
