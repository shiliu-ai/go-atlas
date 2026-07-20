package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// dropConnServer 返回一个 httptest.Server：每次请求先计数，然后 hijack 底层
// 连接并直接关闭（不写任何响应），令客户端的 http.Client.Do 返回连接级错误。
// 这样可确定性地触发 Do() 重试循环里的 `err != nil` 分支。
func dropConnServer(t *testing.T, hits *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Errorf("responsewriter does not support hijack")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = conn.Close() // 不响应直接断开 -> 客户端看到传输层错误
	}))
}

// TestDo_ConnectionError_RetryGatedByMethod 锁定 A1 行为：连接级错误只对幂等
// 方法重试。GET（幂等）在 MaxRetries=2 下应访问服务端 3 次；POST（非幂等）
// 只应访问 1 次——不得重投，否则会造成跨服务重复副作用。
func TestDo_ConnectionError_RetryGatedByMethod(t *testing.T) {
	cases := []struct {
		name         string
		method       string
		wantAttempts int32
	}{
		{"GET retries on connection error", http.MethodGet, 3},
		{"POST does not retry on connection error", http.MethodPost, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits int32
			srv := dropConnServer(t, &hits)
			defer srv.Close()

			c := NewClient(Config{
				Timeout:    2 * time.Second,
				MaxRetries: 2,
				RetryWait:  time.Millisecond,
			}, nil)

			req, err := http.NewRequestWithContext(context.Background(), tc.method, srv.URL, nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}

			if _, err := c.Do(context.Background(), req); err == nil {
				t.Fatal("expected error from dropped connection, got nil")
			}

			if got := atomic.LoadInt32(&hits); got != tc.wantAttempts {
				t.Fatalf("%s attempts = %d, want %d", tc.method, got, tc.wantAttempts)
			}
		})
	}
}

func TestNewClient_TransportPoolDefaults(t *testing.T) {
	c := NewClient(Config{}, nil)

	tr, ok := c.http.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.http.Transport)
	}
	if tr.MaxIdleConns != 256 {
		t.Errorf("MaxIdleConns = %d, want 256", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 64 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 64", tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", tr.IdleConnTimeout)
	}
	if tr.MaxConnsPerHost != 0 {
		t.Errorf("MaxConnsPerHost = %d, want 0 (unlimited)", tr.MaxConnsPerHost)
	}
	// A custom DialContext disables HTTP/2 unless ForceAttemptHTTP2 is set;
	// guard that non-obvious correctness setting against future regressions.
	if !tr.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 = false, want true (custom DialContext otherwise disables HTTP/2)")
	}
}

func TestNewClient_TransportPoolOverrides(t *testing.T) {
	c := NewClient(Config{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		MaxConnsPerHost:     7,
		IdleConnTimeout:     30 * time.Second,
	}, nil)

	tr, ok := c.http.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.http.Transport)
	}
	if tr.MaxIdleConns != 10 {
		t.Errorf("MaxIdleConns = %d, want 10", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 5 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 5", tr.MaxIdleConnsPerHost)
	}
	if tr.MaxConnsPerHost != 7 {
		t.Errorf("MaxConnsPerHost = %d, want 7", tr.MaxConnsPerHost)
	}
	if tr.IdleConnTimeout != 30*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 30s", tr.IdleConnTimeout)
	}
}
