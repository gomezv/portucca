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

func TestPlanExistingClaimOutOfBand(t *testing.T) {
	cfg := baseConfig()
	_, err := Plan(Request{
		Config:   cfg,
		Existing: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3000}},
		Prober:   fakeProber{},
	})
	var outOfBand *OutOfBandError
	if !errors.As(err, &outOfBand) {
		t.Fatalf("err = %v, want OutOfBandError", err)
	}
}

func TestPlanExistingClaimOnMainPortIsDuplicate(t *testing.T) {
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	_, err := Plan(Request{
		Config:   cfg,
		Existing: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3000}},
		Occupied: map[domain.Port]string{3000: "main"},
		Prober:   fakeProber{},
	})
	var duplicate *DuplicateClaimError
	if !errors.As(err, &duplicate) {
		t.Fatalf("err = %v, want DuplicateClaimError", err)
	}
	var outOfBand *OutOfBandError
	if errors.As(err, &outOfBand) {
		t.Fatalf("err = %v, want duplicate before band", err)
	}
}

func TestPlanExistingClaimInvalidPort(t *testing.T) {
	cfg := baseConfig()
	_, err := Plan(Request{
		Config:   cfg,
		Existing: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 80}},
		Prober:   fakeProber{},
	})
	var outOfBand *OutOfBandError
	if !errors.As(err, &outOfBand) {
		t.Fatalf("err = %v, want OutOfBandError", err)
	}
}

func TestPlanDuplicateClaimSamePort(t *testing.T) {
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"api": {MainPort: 4000, StartPort: 3001},
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	_, err := Plan(Request{
		Config: cfg,
		Existing: map[domain.ServiceName]domain.ServiceClaim{
			"api": {Port: 3005},
			"web": {Port: 3005},
		},
		Prober: fakeProber{},
	})
	var duplicate *DuplicateClaimError
	if !errors.As(err, &duplicate) {
		t.Fatalf("err = %v, want DuplicateClaimError", err)
	}
}

func TestPlanExistingClaimOccupied(t *testing.T) {
	cfg := baseConfig()
	_, err := Plan(Request{
		Config:   cfg,
		Existing: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3005}},
		Occupied: map[domain.Port]string{3005: "/other"},
		Prober:   fakeProber{},
	})
	var duplicate *DuplicateClaimError
	if !errors.As(err, &duplicate) {
		t.Fatalf("err = %v, want DuplicateClaimError", err)
	}
}

func TestPlanAdoptionConflict(t *testing.T) {
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"api": {MainPort: 4000, StartPort: 3001, AdoptFrom: &domain.EnvRef{File: ".env.api", Key: "PORT"}},
			"web": {MainPort: 3000, StartPort: 3001, AdoptFrom: &domain.EnvRef{File: ".env.web", Key: "PORT"}},
		},
	}
	_, err := Plan(Request{
		Config: cfg,
		ReadPort: func(domain.EnvRef) (domain.Port, bool, error) {
			return 3010, true, nil
		},
		Prober: fakeProber{},
	})
	var conflict *AdoptionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want AdoptionConflictError", err)
	}
}

func TestPlanDiscardsAdoptionWhenExcludedOrOccupied(t *testing.T) {
	tests := []struct {
		name     string
		exclude  map[domain.ServiceName]domain.Port
		occupied map[domain.Port]string
	}{
		{name: "excluded", exclude: map[domain.ServiceName]domain.Port{"web": 3007}},
		{name: "occupied", occupied: map[domain.Port]string{3007: "/other"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			got, err := Plan(Request{
				Config:   cfg,
				Exclude:  tt.exclude,
				Occupied: tt.occupied,
				ReadPort: func(domain.EnvRef) (domain.Port, bool, error) {
					return 3007, true, nil
				},
				Prober: fakeProber{},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.Services["web"].Port != 3001 || got.Services["web"].Adopted {
				t.Fatalf("got = %+v, want allocated port 3001", got)
			}
		})
	}
}

func TestPlanDiscardsInvalidAdoption(t *testing.T) {
	cfg := baseConfig()
	got, err := Plan(Request{
		Config: cfg,
		ReadPort: func(domain.EnvRef) (domain.Port, bool, error) {
			return 70000, true, nil
		},
		Prober: fakeProber{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Services["web"].Port != 3001 || got.Services["web"].Adopted || !got.Changed {
		t.Fatalf("got = %+v", got)
	}
}
