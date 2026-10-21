package linuxcollect

import (
	"ainfra/internal/domain"
	"context"
	"errors"
	"strings"
	"syscall"
)

type Mount struct {
	Path       string `json:"path"`
	Filesystem string `json:"filesystem"`
}
type Disk struct {
	Mount
	CapacityBytes  uint64 `json:"capacity_bytes"`
	FreeBytes      uint64 `json:"free_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
}

func LocalMounts(raw []byte) ([]Mount, error) {
	out := []Mount{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue
		}
		left, right, ok := strings.Cut(line, " - ")
		if !ok {
			return nil, errors.New("mountinfo_format")
		}
		fields := strings.Fields(left)
		fs := strings.Fields(right)
		if len(fields) < 6 || len(fs) < 3 {
			return nil, errors.New("mountinfo_format")
		}
		switch fs[0] {
		case "ext2", "ext3", "ext4", "xfs", "btrfs", "tmpfs", "overlay", "zfs":
		default:
			continue
		}
		path := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(fields[4])
		if seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, Mount{path, fs[0]})
		if len(out) > 1024 {
			return nil, errors.New("mount_limit")
		}
	}
	return out, nil
}
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
