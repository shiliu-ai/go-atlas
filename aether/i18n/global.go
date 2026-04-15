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
//
// When you need to distinguish a real translation from the key-echo
// fallback (e.g. to log missing translations), use TLookup instead.
func T(ctx context.Context, key string, args ...any) string {
	msg, _ := TLookup(ctx, key)
	return applyArgs(msg, args)
}

// TLookup is like T but reports whether the key was found in any registered
// locale (including the default-language fallback and the built-in English
// messages when no bundle is configured).
//
// When found is false, msg is the key itself. No formatting is applied;
// callers who want fmt.Sprintf behaviour should call applyArgs themselves
// or use T.
//
// Use this to alert on missing translations without parsing T's return value.
func TLookup(ctx context.Context, key string) (msg string, found bool) {
	b := Global()
	if b == nil {
		// No bundle configured; try built-in English messages.
		if m, ok := MessagesEN[key]; ok {
			return m, true
		}
		return key, false
	}
	lang := LocaleFromContext(ctx)
	if lang == language.Und {
		lang = b.DefaultLang()
	}
	// Language from context is already matched by middleware, use direct lookup.
	return b.Lookup(lang, key)
}
