package linuxcollect

import (
	"errors"
	"strings"
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
