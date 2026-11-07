package scan

import (
	"context"

	"github.com/gomezv/portucca/internal/domain"
)

type Scanner struct{}

func (Scanner) Scan(ctx context.Context, opts Options) (Detected, error) {
	return Scan(ctx, opts)
}

func Scan(ctx context.Context, opts Options) (Detected, error) {
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = DefaultMaxEntries
	}
	if opts.MaxFileBytes <= 0 {
		opts.MaxFileBytes = DefaultMaxFileBytes
	}
	exclude := make(map[domain.ServiceName]bool, len(opts.ExcludeServices))
	for _, raw := range opts.ExcludeServices {
		if name := NormalizeServiceName(raw); name != "" {
			exclude[name] = true
		}
	}
	s := &scanner{
		opts:         opts,
		ctx:          ctx,
		byDir:        map[string]*candidate{},
		packages:     map[string]packageInfo{},
		composeNames: map[domain.ServiceName]bool{},
		exclude:      exclude,
	}
	if err := s.walk(ctx, s.detectFile); err != nil {
		return Detected{}, err
	}
	return s.result(), nil
}
