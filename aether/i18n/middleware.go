package i18n

import (
	"github.com/gin-gonic/gin"
	"golang.org/x/text/language"
)

// Middleware extracts the preferred language from the request and stores it
// in the context. Language is resolved in priority order:
//  1. "lang" query parameter
//  2. "X-Language" header
//  3. "Accept-Language" header
//  4. Bundle's default language
func Middleware(b *Bundle) gin.HandlerFunc {
	return func(c *gin.Context) {
		lang := detectLanguage(c, b)
		ctx := WithLocale(c.Request.Context(), lang)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func detectLanguage(c *gin.Context, b *Bundle) language.Tag {
	// 1. Query parameter.
	if q := c.Query("lang"); q != "" {
		if tag, err := language.Parse(q); err == nil {
			return b.Match(tag)
		}
	}

	// 2. X-Language header.
	if h := c.GetHeader("X-Language"); h != "" {
		if tag, err := language.Parse(h); err == nil {
			return b.Match(tag)
		}
	}

	// 3. Accept-Language header.
	if accept := c.GetHeader("Accept-Language"); accept != "" {
		tags, _, err := language.ParseAcceptLanguage(accept)
		if err == nil && len(tags) > 0 {
			return b.Match(tags...)
		}
	}

	return b.DefaultLang()
}
