package database

import (
	"context"

	"gorm.io/gorm"
)

type contextKey struct{ name string }

// WithTx stores a gorm.DB transaction in context for the default database.
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return WithNamedTx(ctx, DefaultName, tx)
}

// WithNamedTx stores a gorm.DB transaction in context for the named database.
func WithNamedTx(ctx context.Context, name string, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, contextKey{name: name}, tx)
}

// TxFromContext retrieves the gorm.DB transaction from context for the default database.
func TxFromContext(ctx context.Context) *gorm.DB {
	return NamedTxFromContext(ctx, DefaultName)
}

// NamedTxFromContext retrieves the gorm.DB transaction from context for the named database.
func NamedTxFromContext(ctx context.Context, name string) *gorm.DB {
	tx, _ := ctx.Value(contextKey{name: name}).(*gorm.DB)
	return tx
}

// Transaction runs fn within a database transaction on the default database.
func Transaction(ctx context.Context, db *gorm.DB, fn func(ctx context.Context) error) error {
	return NamedTransaction(ctx, DefaultName, db, fn)
}

// NamedTransaction runs fn within a database transaction, storing the tx under the given name.
func NamedTransaction(ctx context.Context, name string, db *gorm.DB, fn func(ctx context.Context) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := WithNamedTx(ctx, name, tx)
		return fn(txCtx)
	})
}
