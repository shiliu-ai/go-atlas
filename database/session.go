package database

import (
	"context"

	"gorm.io/gorm"
)

type contextKey struct{}

// WithTx stores a gorm.DB transaction in context for transaction propagation.
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, contextKey{}, tx)
}

// TxFromContext retrieves the gorm.DB transaction from context.
func TxFromContext(ctx context.Context) *gorm.DB {
	tx, _ := ctx.Value(contextKey{}).(*gorm.DB)
	return tx
}

// Transaction runs fn within a database transaction.
func Transaction(ctx context.Context, db *gorm.DB, fn func(ctx context.Context) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := WithTx(ctx, tx)
		return fn(txCtx)
	})
}
