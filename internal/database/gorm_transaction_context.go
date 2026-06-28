package database

import (
	"context"

	"gorm.io/gorm"
)

type gormTransactionContextKey struct{}

// WithGormTransaction stores the current GORM transaction in ctx so
// repositories that are not directly handed the tx can still join the
// caller's transaction.
func WithGormTransaction(ctx context.Context, tx *gorm.DB) context.Context {
	if ctx == nil || tx == nil {
		return ctx
	}
	return context.WithValue(ctx, gormTransactionContextKey{}, tx)
}

// GormTransactionFromContext returns a transaction previously attached
// with WithGormTransaction.
func GormTransactionFromContext(ctx context.Context) (*gorm.DB, bool) {
	if ctx == nil {
		return nil, false
	}
	tx, ok := ctx.Value(gormTransactionContextKey{}).(*gorm.DB)
	return tx, ok && tx != nil
}
