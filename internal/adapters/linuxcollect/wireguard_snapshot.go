package linuxcollect

import (
	"ainfra/internal/domain"
	"bytes"
	"encoding/json"
	"time"
)

func (c *Collector) readWireGuardSnapshot() domain.Section {
	raw, err := c.read(c.wireguardSnapshot)
	if err != nil || len(raw) > 1<<20 || !json.Valid(raw) {
		return failed("snapshot_unavailable")
	}
	var section domain.Section
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&section) != nil || len(section.ErrorCode) > 128 {
		return failed("snapshot_invalid")
	}
	if section.Status == "error" && len(section.Data) == 0 {
		return section
	}
	if section.Status != "ok" || !section.Complete || section.ObservedAt.IsZero() || section.ObservedAt.After(c.now().Add(5*time.Minute)) {
		return failed("snapshot_invalid")
	}
	var fields map[string]string
	if json.Unmarshal(section.Data, &fields) != nil || len(fields) != 5 {
		return failed("snapshot_invalid")
	}
	for key := range fields {
		switch key {
		case "public-key", "peers", "endpoints", "allowed-ips", "latest-handshakes":
		default:
			return failed("snapshot_invalid")
		}
	}
	return section
}
