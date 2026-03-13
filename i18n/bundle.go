package i18n

import (
	"fmt"

	"golang.org/x/text/language"
)

// Bundle holds translations for multiple languages and provides
// message lookup with best-match language negotiation.
//
// All Register calls must happen before the bundle is used for translation
// (i.e., before serving requests). After registration is complete, all
// methods are safe for concurrent use without locking.
type Bundle struct {
	defaultLang language.Tag
	supported   []language.Tag
	matcher     language.Matcher
	messages    map[language.Tag]map[string]string
}

// NewBundle creates a new translation bundle with the given default language.
func NewBundle(defaultLang language.Tag) *Bundle {
	return &Bundle{
		defaultLang: defaultLang,
		supported:   []language.Tag{defaultLang},
		matcher:     language.NewMatcher([]language.Tag{defaultLang}),
		messages:    make(map[language.Tag]map[string]string),
	}
}

// Register adds or merges messages for the given language.
// Existing keys are overwritten.
//
// Must be called before the bundle is used for translation (before serving).
func (b *Bundle) Register(lang language.Tag, messages map[string]string) {
	if _, ok := b.messages[lang]; !ok {
		b.messages[lang] = make(map[string]string, len(messages))
		if lang != b.defaultLang {
			b.supported = append(b.supported, lang)
			b.matcher = language.NewMatcher(b.supported)
		}
	}

	for k, v := range messages {
		b.messages[lang][k] = v
	}
}

// DefaultLang returns the bundle's default language.
func (b *Bundle) DefaultLang() language.Tag {
	return b.defaultLang
}

// Match returns the best supported language for the given tags.
func (b *Bundle) Match(preferred ...language.Tag) language.Tag {
	tag, _, _ := b.matcher.Match(preferred...)
	return tag
}

// T translates a message key by first matching the best supported language,
// then looking up the translation. Use Translate when the language tag is
// already matched (e.g., from middleware).
func (b *Bundle) T(lang language.Tag, key string, args ...any) string {
	matched, _, _ := b.matcher.Match(lang)
	return b.Translate(matched, key, args...)
}

// Translate looks up a message by an already-matched language tag.
// Falls back to the default language, then to the key itself.
func (b *Bundle) Translate(lang language.Tag, key string, args ...any) string {
	if msgs, ok := b.messages[lang]; ok {
		if msg, ok := msgs[key]; ok {
			return applyArgs(msg, args)
		}
	}

	// Fallback to default language.
	if lang != b.defaultLang {
		if msgs, ok := b.messages[b.defaultLang]; ok {
			if msg, ok := msgs[key]; ok {
				return applyArgs(msg, args)
			}
		}
	}

	// Key itself as last resort.
	return applyArgs(key, args)
}

func applyArgs(msg string, args []any) string {
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}
