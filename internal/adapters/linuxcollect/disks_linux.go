package linuxcollect

import (
	"ainfra/internal/domain"
	"context"
	"syscall"
)

func (c *Collector) Disks(ctx context.Context) domain.Section {
	b, e := c.read("/proc/self/mountinfo")
	if e != nil {
		return failed("mountinfo_unavailable")
	}
	mounts, e := LocalMounts(b)
	if e != nil {
		return failed("mountinfo_format")
	}
	rows := []Disk{}
	partial := false
	for _, m := range mounts {
		if ctx.Err() != nil {
			return failed("timeout")
		}
		var st syscall.Statfs_t
		if e = syscall.Statfs(m.Path, &st); e != nil {
			partial = true
			continue
		}
		rows = append(rows, Disk{Mount: m, CapacityBytes: st.Blocks * uint64(st.Bsize), FreeBytes: st.Bfree * uint64(st.Bsize), AvailableBytes: st.Bavail * uint64(st.Bsize)})
	}
	s := c.section(rows)
	if partial {
		s.Status = "partial"
		s.Complete = false
		s.ErrorCode = "statfs_failed"
	}
	return s
}
