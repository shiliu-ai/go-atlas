package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
"sync"
	"time"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

type colorHandler struct {
	opts  slog.HandlerOptions
	out   io.Writer
	mu    *sync.Mutex
	attrs []slog.Attr
	group string
}

func newColorHandler(out io.Writer, opts *slog.HandlerOptions) *colorHandler {
	h := &colorHandler{out: out, mu: &sync.Mutex{}}
	if opts != nil {
		h.opts = *opts
	}
	return h
}

func (h *colorHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *colorHandler) Handle(_ context.Context, r slog.Record) error {
	levelColor, levelText := levelStyle(r.Level)

	var buf []byte
	buf = append(buf, colorGray...)
	buf = append(buf, r.Time.Format(time.DateTime)...)
	buf = append(buf, colorReset...)
	buf = append(buf, ' ')
	buf = append(buf, levelColor...)
	buf = append(buf, levelText...)
	buf = append(buf, colorReset...)
	buf = append(buf, ' ')
	buf = append(buf, r.Message...)

	// pre-set attrs
	for _, a := range h.attrs {
		buf = appendAttr(buf, a)
	}
	// record attrs
	r.Attrs(func(a slog.Attr) bool {
		buf = appendAttr(buf, a)
		return true
	})

	buf = append(buf, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write(buf)
	return err
}

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &colorHandler{opts: h.opts, out: h.out, mu: h.mu, attrs: newAttrs, group: h.group}
}

func (h *colorHandler) WithGroup(name string) slog.Handler {
	return &colorHandler{opts: h.opts, out: h.out, mu: h.mu, attrs: h.attrs, group: name}
}

func levelStyle(level slog.Level) (color string, text string) {
	switch {
	case level >= slog.LevelError:
		return colorRed, "ERR"
	case level >= slog.LevelWarn:
		return colorYellow, "WRN"
	case level >= slog.LevelInfo:
		return colorGreen, "INF"
	default:
		return colorCyan, "DBG"
	}
}

func appendAttr(buf []byte, a slog.Attr) []byte {
	if a.Equal(slog.Attr{}) {
		return buf
	}
	buf = append(buf, ' ')
	buf = append(buf, colorCyan...)
	buf = append(buf, a.Key...)
	buf = append(buf, colorReset...)
	buf = append(buf, '=')
	buf = append(buf, fmt.Sprintf("%v", a.Value.Any())...)
	return buf
}

// Ensure colorHandler implements slog.Handler.
var _ slog.Handler = (*colorHandler)(nil)
