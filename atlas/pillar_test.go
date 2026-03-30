package atlas

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/shiliu-ai/go-atlas/log"
)

// --- test doubles ---

type testPillar struct {
	name    string
	inited  bool
	stopped bool
}

func (p *testPillar) Name() string                   { return p.name }
func (p *testPillar) Init(core *Core) error          { p.inited = true; return nil }
func (p *testPillar) Stop(ctx context.Context) error { p.stopped = true; return nil }

type testHealthPillar struct {
	testPillar
	healthy bool
}

func (p *testHealthPillar) Health(ctx context.Context) error {
	if !p.healthy {
		return fmt.Errorf("unhealthy")
	}
	return nil
}

// --- PillarRegistry tests ---

func TestRegister(t *testing.T) {
	r := NewPillarRegistry()
	p := &testPillar{name: "test"}
	r.Register(p)

	pillars := r.Pillars()
	if len(pillars) != 1 {
		t.Fatalf("expected 1 pillar, got %d", len(pillars))
	}
	if pillars[0].Name() != "test" {
		t.Fatal("pillar not registered with correct name")
	}
}

func TestRegisterDuplicate(t *testing.T) {
	r := NewPillarRegistry()
	r.Register(&testPillar{name: "test"})

	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	r.Register(&testPillar{name: "test"})
}

func TestRegisterOrder(t *testing.T) {
	r := NewPillarRegistry()
	r.Register(&testPillar{name: "a"})
	r.Register(&testPillar{name: "b"})
	r.Register(&testPillar{name: "c"})

	pillars := r.Pillars()
	if len(pillars) != 3 {
		t.Fatalf("expected 3 pillars, got %d", len(pillars))
	}
	for i, want := range []string{"a", "b", "c"} {
		if pillars[i].Name() != want {
			t.Fatalf("pillar[%d]: want %q, got %q", i, want, pillars[i].Name())
		}
	}
}

func TestUsePillar(t *testing.T) {
	r := NewPillarRegistry()
	p := &testPillar{name: "test"}
	r.Register(p)

	got := UsePillar[*testPillar](r)
	if got != p {
		t.Fatal("UsePillar returned wrong pillar")
	}
}

func TestUsePillarPanic(t *testing.T) {
	r := NewPillarRegistry()

	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("expected panic on missing pillar")
		}
	}()
	UsePillar[*testPillar](r)
}

func TestTryUsePillar(t *testing.T) {
	r := NewPillarRegistry()
	r.Register(&testPillar{name: "test"})

	got, ok := TryUsePillar[*testPillar](r)
	if !ok || got == nil {
		t.Fatal("TryUsePillar should find registered pillar")
	}

	_, ok = TryUsePillar[*testHealthPillar](r)
	if ok {
		t.Fatal("TryUsePillar should return false for unregistered type")
	}
}

// --- Interface compliance tests ---

func TestPillarInterface(t *testing.T) {
	var _ Pillar = (*testPillar)(nil)
}

func TestHealthCheckerInterface(t *testing.T) {
	var _ HealthChecker = (*testHealthPillar)(nil)
}

func TestHealthCheckerBehavior(t *testing.T) {
	p := &testHealthPillar{testPillar: testPillar{name: "hc"}, healthy: false}
	if err := p.Health(context.Background()); err == nil {
		t.Fatal("expected error for unhealthy pillar")
	}
	p.healthy = true
	if err := p.Health(context.Background()); err != nil {
		t.Fatalf("expected no error for healthy pillar, got %v", err)
	}
}

// --- Core tests ---

func TestCoreUnmarshal(t *testing.T) {
	v := viper.New()
	v.Set("db.host", "localhost")
	v.Set("db.port", 5432)

	c := NewCore(v, nil)

	var cfg struct {
		Host string `mapstructure:"host"`
		Port int    `mapstructure:"port"`
	}
	if err := c.Unmarshal("db", &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Host != "localhost" || cfg.Port != 5432 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestCoreUnmarshalMissing(t *testing.T) {
	v := viper.New()
	c := NewCore(v, nil)

	var cfg struct{}
	err := c.Unmarshal("nonexistent", &cfg)
	if err == nil {
		t.Fatal("expected error for missing config section")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("error should mention the key: %v", err)
	}
}

func TestCoreLogger(t *testing.T) {
	v := viper.New()
	l := log.NewDefault(log.LevelInfo)
	c := NewCore(v, l)

	sub := c.Logger("mymod")
	if sub == nil {
		t.Fatal("Logger returned nil")
	}
}
