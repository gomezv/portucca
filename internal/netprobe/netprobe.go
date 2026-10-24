package netprobe

import (
	"fmt"

	"github.com/gomezv/portucca/internal/domain"
)

type Prober struct{}

func (Prober) IsFree(port domain.Port) bool {
	if !port.Valid() {
		return false
	}
	if !tryListen("tcp4", fmt.Sprintf("0.0.0.0:%d", port)) {
		return false
	}
	if !tryListen("tcp6", fmt.Sprintf("[::]:%d", port)) {
		return false
	}
	return true
}
