package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

// loadCLIConfig is the single config-load path for every verb. config.Load
// already fails loud with the KERNL DISPATCH FAILURE marker; call sites used
// to re-wrap it (producing a double marker) and none named the recovery. The
// most common failure - running kernl outside the repo root - now carries its
// Fix inline, including the permanent recovery: a caller who hits this once
// hits it on every invocation, and cd-ing back is a workaround, not a fix.
func loadCLIConfig(path string) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w - Fix: pass --config <path-to-kernl.yaml>, run kernl from the directory containing %s, or export KERNL_CONFIG=<path-to-kernl.yaml> to stop passing it", err, path)
		}
		return nil, err
	}
	return cfg, nil
}
