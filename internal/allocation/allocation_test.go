package allocation

import (
	"errors"
	"testing"

	"github.com/gomezv/portucca/internal/domain"
)

type fakeProber struct{ busy map[domain.Port]bool }

func (f fakeProber) IsFree(p domain.Port) bool { return !f.busy[p] }

func baseConfig() domain.Config {
	ref := domain.EnvRef{File: ".env.local", Key: "PORT"}
	return domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001, AdoptFrom: &ref},
		},
	}
}

func TestPlanKeepsExisting(t *testing.T) {
	cfg := baseConfig()
	got, err := Plan(Request{
		Config:   cfg,
		Existing: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3005}},
		Prober:   fakeProber{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Services["web"].Port != 3005 || got.Changed {
		t.Fatalf("got = %+v", got)
	}
}

func TestPlanAdoptsExistingEnvPort(t *testing.T) {
	cfg := baseConfig()
	got, err := Plan(Request{
		Config: cfg,
		ReadPort: func(domain.EnvRef) (domain.Port, bool, error) {
			return 3007, true, nil
		},
		Prober: fakeProber{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Services["web"].Port != 3007 || !got.Services["web"].Adopted || !got.Changed {
		t.Fatalf("got = %+v", got)
	}
}

func TestPlanSkipsOccupiedAndBusy(t *testing.T) {
	cfg := baseConfig()
	got, err := Plan(Request{
		Config:   cfg,
		Occupied: map[domain.Port]string{3001: "/other"},
		Prober:   fakeProber{busy: map[domain.Port]bool{3002: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Services["web"].Port != 3003 {
		t.Fatalf("port = %d, want 3003", got.Services["web"].Port)
	}
}

func TestPlanExhausted(t *testing.T) {
	cfg := baseConfig()
	cfg.Services["web"] = domain.Service{MainPort: 65534, StartPort: 65535}
	_, err := Plan(Request{Config: cfg, Prober: fakeProber{busy: map[domain.Port]bool{65535: true}}})
	var exhausted *PortExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("err = %v, want PortExhaustedError", err)
	}
}

func TestPlanExcludesOldPortsForReallocate(t *testing.T) {
	cfg := baseConfig()
	got, err := Plan(Request{
		Config:  cfg,
		Exclude: map[domain.ServiceName]domain.Port{"web": 3001},
		Prober:  fakeProber{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Services["web"].Port != 3002 {
		t.Fatalf("port = %d, want 3002", got.Services["web"].Port)
	}
}
