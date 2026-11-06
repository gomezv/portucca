package scan

import "context"

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
	s := &scanner{
		opts:     opts,
		ctx:      ctx,
		byDir:    map[string]*candidate{},
		packages: map[string]packageInfo{},
	}
	if err := s.walk(ctx, s.detectFile); err != nil {
		return Detected{}, err
	}
	return s.result(), nil
}
