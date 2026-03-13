package i18n

import (
	"context"

	"golang.org/x/text/language"
)

type contextKey struct{}

// WithLocale returns a new context with the given language tag.
func WithLocale(ctx context.Context, lang language.Tag) context.Context {
	return context.WithValue(ctx, contextKey{}, lang)
}

// LocaleFromContext extracts the language tag from the context.
// Returns language.Und if not set.
func LocaleFromContext(ctx context.Context) language.Tag {
	if lang, ok := ctx.Value(contextKey{}).(language.Tag); ok {
		return lang
	}
	return language.Und
}
