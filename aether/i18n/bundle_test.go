package i18n

import (
	"context"
	"testing"

	"golang.org/x/text/language"
)

func newTestBundle(t *testing.T) *Bundle {
	t.Helper()
	b := NewBundle(language.English)
	b.Register(language.English, map[string]string{
		"greeting": "Hello, %s",
		"title":    "Welcome",
	})
	b.Register(language.Japanese, map[string]string{
		"greeting": "こんにちは、%s",
		// "title" intentionally missing — should fall back to English.
	})
	return b
}

func TestBundle_Lookup_Found(t *testing.T) {
	b := newTestBundle(t)
	msg, found := b.Lookup(language.Japanese, "greeting")
	if !found {
		t.Fatal("expected found=true for registered key")
	}
	if msg != "こんにちは、%s" {
		t.Errorf("msg = %q, want %q", msg, "こんにちは、%s")
	}
}

func TestBundle_Lookup_FallsBackToDefault(t *testing.T) {
	b := newTestBundle(t)
	msg, found := b.Lookup(language.Japanese, "title")
	if !found {
		t.Fatal("expected found=true via default-language fallback")
	}
	if msg != "Welcome" {
		t.Errorf("msg = %q, want %q (English fallback)", msg, "Welcome")
	}
}

func TestBundle_Lookup_MissingReturnsKey(t *testing.T) {
	b := newTestBundle(t)
	msg, found := b.Lookup(language.English, "nope.key")
	if found {
		t.Fatal("expected found=false for unregistered key")
	}
	if msg != "nope.key" {
		t.Errorf("msg = %q, want key echoed back", msg)
	}
}

func TestBundle_Lookup_UnknownLangFallsBackToDefault(t *testing.T) {
	b := newTestBundle(t)
	// French is not registered at all — direct Lookup should still resolve
	// via the default-language map (English).
	msg, found := b.Lookup(language.French, "greeting")
	if !found {
		t.Fatal("expected found=true via default-language fallback")
	}
	if msg != "Hello, %s" {
		t.Errorf("msg = %q, want English fallback %q", msg, "Hello, %s")
	}
}

func TestBundle_TLookup_MatchesVariantToRegistered(t *testing.T) {
	// Only register a regional English variant. TLookup on plain English
	// should resolve to it via the matcher; Lookup on plain English would
	// miss and fall back to the default-language map (same one here, so
	// both paths resolve — we use BritishEnglish to make the test
	// meaningful).
	b := NewBundle(language.English)
	b.Register(language.BritishEnglish, map[string]string{
		"colour": "colour",
	})

	// TLookup negotiates — en-GB wins for a British-specific query even
	// when only en-GB is registered.
	msg, found := b.TLookup(language.BritishEnglish, "colour")
	if !found {
		t.Fatalf("TLookup(en-GB) found=false, want true")
	}
	if msg != "colour" {
		t.Errorf("msg = %q, want %q", msg, "colour")
	}
}

func TestBundle_Translate_MissingKeyEchoesKey(t *testing.T) {
	// Backwards-compat: Translate still returns the key itself when missing.
	b := newTestBundle(t)
	got := b.Translate(language.English, "missing.key")
	if got != "missing.key" {
		t.Errorf("Translate = %q, want key echo", got)
	}
}

func TestBundle_Translate_AppliesArgs(t *testing.T) {
	b := newTestBundle(t)
	got := b.Translate(language.English, "greeting", "world")
	if got != "Hello, world" {
		t.Errorf("got %q, want %q", got, "Hello, world")
	}
}

func TestGlobal_TLookup_BundleUnset(t *testing.T) {
	prev := Global()
	SetGlobal(nil)
	t.Cleanup(func() { SetGlobal(prev) })

	// With no bundle, TLookup consults built-in MessagesEN (if the key
	// exists there) and otherwise reports found=false.
	msg, found := TLookup(context.Background(), "totally.unknown.key")
	if found {
		t.Errorf("expected found=false, got msg=%q", msg)
	}
	if msg != "totally.unknown.key" {
		t.Errorf("msg = %q, want key echo", msg)
	}
}

func TestGlobal_TLookup_UsesContextLocale(t *testing.T) {
	b := newTestBundle(t)
	SetGlobal(b)
	t.Cleanup(func() { SetGlobal(nil) })

	ctx := WithLocale(context.Background(), language.Japanese)
	msg, found := TLookup(ctx, "greeting")
	if !found {
		t.Fatal("expected found=true")
	}
	if msg != "こんにちは、%s" {
		t.Errorf("msg = %q, want Japanese", msg)
	}
}
