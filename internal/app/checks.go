package app

import (
	"ainfra/internal/domain"
	"bytes"
	"encoding/json"
	"time"
)

type Check struct {
	SchemaVersion int             `json:"schema_version"`
	ID            string          `json:"check_id"`
	Kind          string          `json:"kind"`
	ObservedAt    time.Time       `json:"observed_at"`
	Status        string          `json:"status"`
	Data          json.RawMessage `json:"data"`
}
type BackupResult struct {
	LastAttempt       time.Time  `json:"last_attempt_at"`
	LastAttemptStatus string     `json:"last_attempt_status"`
	LastSuccess       *time.Time `json:"last_success_at"`
	LastRestoreTest   *time.Time `json:"last_restore_test_at"`
	Evidence          string     `json:"evidence"`
}
type PaymentResult struct {
	NextDue  time.Time `json:"next_due_date"`
	Currency string    `json:"currency"`
	Amount   string    `json:"amount"`
	Evidence string    `json:"evidence"`
}

func strict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return ErrInvalid
	}
	return nil
}
func ValidateChecks(b json.RawMessage) error {
	var cs []Check
	if strict(b, &cs) != nil || len(cs) > 1000 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, c := range cs {
		if c.SchemaVersion != 1 || c.ID == "" || len(c.ID) > 200 || seen[c.ID] || c.ObservedAt.IsZero() || !contains([]string{"ok", "error"}, c.Status) {
			return ErrInvalid
		}
		seen[c.ID] = true
		switch c.Kind {
		case "backup":
			var v BackupResult
			if strict(c.Data, &v) != nil || v.LastAttempt.IsZero() || v.LastAttempt.After(c.ObservedAt) || !contains([]string{"success", "error"}, v.LastAttemptStatus) || len(v.Evidence) > 4096 {
				return ErrInvalid
			}
			if v.LastSuccess != nil && v.LastSuccess.After(c.ObservedAt) {
				return ErrInvalid
			}
			if v.LastRestoreTest != nil && v.LastRestoreTest.After(c.ObservedAt) {
				return ErrInvalid
			}
		case "payment":
			var v PaymentResult
			if strict(c.Data, &v) != nil || (c.Status == "ok" && v.NextDue.IsZero()) || len(v.Amount) > 128 || len(v.Currency) > 32 || len(v.Evidence) > 4096 {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
	}
	return nil
}
func applyChecks(n *domain.Node, b json.RawMessage, complete bool) error {
	if e := ValidateChecks(b); e != nil {
		return e
	}
	var cs []Check
	_ = json.Unmarshal(b, &cs)
	if complete {
		current := map[string]bool{}
		for _, c := range cs {
			if c.Kind == "payment" {
				current[c.ID] = true
			}
		}
		for id := range n.PaymentChecks {
			if !current[id] {
				delete(n.PaymentChecks, id)
			}
		}
	}
	for _, c := range cs {
		if c.Kind == "payment" {
			if n.PaymentChecks == nil {
				n.PaymentChecks = map[string]domain.PaymentObservation{}
			}
			if _, exists := n.PaymentChecks[c.ID]; !exists && len(n.PaymentChecks) >= 1000 {
				return ErrInvalid
			}
			previous := n.PaymentChecks[c.ID]
			if c.ObservedAt.Before(previous.ObservedAt) {
				continue
			}
			previous.ObservedAt, previous.Status = c.ObservedAt, c.Status
			if c.Status == "ok" {
				var v PaymentResult
				_ = json.Unmarshal(c.Data, &v)
				previous.NextDue, previous.Currency, previous.Amount = v.NextDue, v.Currency, v.Amount
				previous.Evidence, previous.LastSuccessAt = v.Evidence, c.ObservedAt
			}
			n.PaymentChecks[c.ID] = previous
			continue
		}
		if c.Kind != "backup" {
			continue
		}
		var v BackupResult
		_ = json.Unmarshal(c.Data, &v)
		for i := range n.Backups {
			job := &n.Backups[i]
			if job.ID != c.ID || c.ObservedAt.Before(job.ObservedAt) {
				continue
			}
			job.ObservedAt = c.ObservedAt
			job.LastAttempt = v.LastAttempt
			job.LastAttemptStatus = v.LastAttemptStatus
			if v.LastSuccess != nil {
				job.LastSuccess = *v.LastSuccess
			}
			if v.LastRestoreTest != nil {
				job.LastRestoreTest = *v.LastRestoreTest
			}
			job.Evidence = v.Evidence
		}
	}
	return nil
}
