package i18n

import (
	"context"
	"sync/atomic"

	"golang.org/x/text/language"
)

var globalBundle atomic.Pointer[Bundle]

// SetGlobal sets the global i18n bundle.
func SetGlobal(b *Bundle) {
	globalBundle.Store(b)
}

// Global returns the global i18n bundle, or nil if not set.
func Global() *Bundle {
	return globalBundle.Load()
}

// T translates a message key using the global bundle and locale from context.
// If the global bundle is not set, falls back to built-in English messages,
// then to the key itself.
func T(ctx context.Context, key string, args ...any) string {
	b := Global()
	if b == nil {
		// No bundle configured; try built-in English messages.
		if msg, ok := MessagesEN[key]; ok {
			return applyArgs(msg, args)
		}
		return applyArgs(key, args)
	}
	lang := LocaleFromContext(ctx)
	if lang == language.Und {
		lang = b.DefaultLang()
	}
	// Language from context is already matched by middleware, use direct lookup.
	return b.Translate(lang, key, args...)
}
