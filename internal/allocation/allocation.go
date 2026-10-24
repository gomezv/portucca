package allocation

import (
	"fmt"

	"github.com/gomezv/portucca/internal/domain"
)

type Prober interface {
	IsFree(domain.Port) bool
}

type ReadPort func(domain.EnvRef) (domain.Port, bool, error)

type Request struct {
	Config      domain.Config
	Existing    map[domain.ServiceName]domain.ServiceClaim
	Occupied    map[domain.Port]string
	Exclude     map[domain.ServiceName]domain.Port
	Prober      Prober
	ReadPort    ReadPort
	MaxAttempts int
}

type Result struct {
	Services map[domain.ServiceName]domain.ServiceClaim
	Changed  bool
}

type PortExhaustedError struct {
	Service  domain.ServiceName
	Start    domain.Port
	Attempts int
}

func (e *PortExhaustedError) Error() string {
	return fmt.Sprintf("port_exhausted: service %s from %d after %d attempts", e.Service, e.Start, e.Attempts)
}

type DuplicateClaimError struct{ Port domain.Port }

func (e *DuplicateClaimError) Error() string {
	return fmt.Sprintf("duplicate_claim: port %d", e.Port)
}

type AdoptionConflictError struct{ Port domain.Port }

func (e *AdoptionConflictError) Error() string {
	return fmt.Sprintf("adoption_conflict: port %d", e.Port)
}

type OutOfBandError struct {
	Service domain.ServiceName
	Port    domain.Port
}

func (e *OutOfBandError) Error() string {
	return fmt.Sprintf("claim_out_of_band: service %s port %d", e.Service, e.Port)
}

func Plan(req Request) (Result, error) {
	if req.MaxAttempts <= 0 {
		req.MaxAttempts = domain.DefaultMaxSlots
	}
	result := Result{Services: map[domain.ServiceName]domain.ServiceClaim{}}
	used := map[domain.Port]bool{}
	excluded := map[domain.Port]bool{}
	for _, p := range req.Exclude {
		excluded[p] = true
	}
	for _, name := range req.Config.ServiceNames() {
		claim, ok := req.Existing[name]
		if !ok {
			continue
		}
		svc := req.Config.Services[name]
		if !claim.Port.Valid() || claim.Port < svc.StartPort {
			return Result{}, &OutOfBandError{Service: name, Port: claim.Port}
		}
		if used[claim.Port] {
			return Result{}, &DuplicateClaimError{Port: claim.Port}
		}
		if req.Occupied[claim.Port] != "" {
			return Result{}, &DuplicateClaimError{Port: claim.Port}
		}
		used[claim.Port] = true
		result.Services[name] = claim
	}
	for _, name := range req.Config.ServiceNames() {
		if _, ok := result.Services[name]; ok {
			continue
		}
		svc := req.Config.Services[name]
		if svc.AdoptFrom == nil || req.ReadPort == nil {
			continue
		}
		port, ok, err := req.ReadPort(*svc.AdoptFrom)
		if err != nil {
			return Result{}, err
		}
		if !ok || !port.Valid() || port < svc.StartPort {
			continue
		}
		if used[port] {
			return Result{}, &AdoptionConflictError{Port: port}
		}
		if excluded[port] || req.Occupied[port] != "" {
			continue
		}
		used[port] = true
		result.Services[name] = domain.ServiceClaim{Port: port, Adopted: true}
		result.Changed = true
	}
	for _, name := range req.Config.ServiceNames() {
		if _, ok := result.Services[name]; ok {
			continue
		}
		svc := req.Config.Services[name]
		port, attempts, err := findPort(svc.StartPort, used, excluded, req.Occupied, req.Prober, req.MaxAttempts)
		if err != nil {
			return Result{}, &PortExhaustedError{Service: name, Start: svc.StartPort, Attempts: attempts}
		}
		used[port] = true
		result.Services[name] = domain.ServiceClaim{Port: port}
		result.Changed = true
	}
	return result, nil
}

func findPort(start domain.Port, used, excluded map[domain.Port]bool, occupied map[domain.Port]string, prober Prober, max int) (domain.Port, int, error) {
	attempts := 0
	for p := start; p <= domain.MaxPort && attempts < max; p++ {
		attempts++
		if used[p] || excluded[p] || occupied[p] != "" {
			continue
		}
		if prober != nil && !prober.IsFree(p) {
			continue
		}
		return p, attempts, nil
	}
	return 0, attempts, fmt.Errorf("no free port")
}
