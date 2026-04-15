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
//
// When you need to distinguish a real translation from the key-echo
// fallback (e.g. to log missing translations), use Lookup instead.
func (b *Bundle) Translate(lang language.Tag, key string, args ...any) string {
	msg, _ := b.Lookup(lang, key)
	return applyArgs(msg, args)
}

// Lookup looks up a message by an already-matched language tag and reports
// whether the key was found in any registered locale (including the
// default-language fallback).
//
// When found is false, msg is the key itself — callers that want
// key-echo behaviour can use it verbatim, while callers that want to
// alert on missing translations get an unambiguous signal.
//
// No formatting is applied; callers who want fmt.Sprintf behaviour should
// pass the returned message through their own formatter, or use Translate.
func (b *Bundle) Lookup(lang language.Tag, key string) (msg string, found bool) {
	if msgs, ok := b.messages[lang]; ok {
		if m, ok := msgs[key]; ok {
			return m, true
		}
	}
	if lang != b.defaultLang {
		if msgs, ok := b.messages[b.defaultLang]; ok {
			if m, ok := msgs[key]; ok {
				return m, true
			}
		}
	}
	return key, false
}

// TLookup is the matching variant of Lookup: it first resolves lang to the
// best supported language, then looks up the key. Use this when lang
// hasn't been pre-matched (e.g. a raw Accept-Language header value).
func (b *Bundle) TLookup(lang language.Tag, key string) (msg string, found bool) {
	matched, _, _ := b.matcher.Match(lang)
	return b.Lookup(matched, key)
}

func applyArgs(msg string, args []any) string {
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}
