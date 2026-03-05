package database

import (
	"context"

	"xorm.io/xorm"
)

type contextKey struct{}

// WithSession stores a xorm.Session in context for transaction propagation.
func WithSession(ctx context.Context, sess *xorm.Session) context.Context {
	return context.WithValue(ctx, contextKey{}, sess)
}

// SessionFromContext retrieves the xorm.Session from context.
func SessionFromContext(ctx context.Context) *xorm.Session {
	sess, _ := ctx.Value(contextKey{}).(*xorm.Session)
	return sess
}

// Transaction runs fn within a database transaction.
func Transaction(ctx context.Context, engine *xorm.Engine, fn func(ctx context.Context) error) error {
	sess := engine.NewSession()
	defer sess.Close()

	if err := sess.Begin(); err != nil {
		return err
	}

	txCtx := WithSession(ctx, sess)
	if err := fn(txCtx); err != nil {
		_ = sess.Rollback()
		return err
	}

	return sess.Commit()
}
