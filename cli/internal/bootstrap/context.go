package bootstrap

import "context"

// ctxKey 是 Dependencies 在 Context 中的类型安全 key。
type ctxKey struct{}

// WithDependencies 将 Dependencies 注入到 Context 中。
func WithDependencies(ctx context.Context, deps *Dependencies) context.Context {
	return context.WithValue(ctx, ctxKey{}, deps)
}

// DepsFromContext 从 Context 中提取 Dependencies。
// 如果 Context 中不包含 Dependencies，则 panic（编程错误）。
func DepsFromContext(ctx context.Context) *Dependencies {
	deps, ok := ctx.Value(ctxKey{}).(*Dependencies)
	if !ok || deps == nil {
		panic("bootstrap: Dependencies not found in context, ensure PersistentPreRunE has been executed")
	}
	return deps
}
