//go:build !linux

package linuxcollect

import (
	"ainfra/internal/domain"
	"context"
)

func (c *Collector) Disks(context.Context) domain.Section {
	return domain.Section{Status: "unsupported", ErrorCode: "linux_required"}
}
