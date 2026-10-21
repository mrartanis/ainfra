package domain

import (
	"encoding/json"
	"time"
)

type Principal struct {
	ID       string   `json:"id"`
	Role     string   `json:"role"`
	Nodes    []string `json:"nodes,omitempty"`
	Instance string   `json:"instance,omitempty"`
}
type Token struct {
	ID        string    `json:"id"`
	Digest    string    `json:"-"`
	Principal Principal `json:"principal"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
}
type Section struct {
	Status     string          `json:"status"`
	Complete   bool            `json:"complete"`
	ObservedAt time.Time       `json:"observed_at"`
	Data       json.RawMessage `json:"data,omitempty"`
	ErrorCode  string          `json:"error_code,omitempty"`
}
type Report struct {
	SchemaVersion int                `json:"schema_version"`
	ID            string             `json:"report_id"`
	NodeID        string             `json:"node_id"`
	Instance      string             `json:"agent_instance_id"`
	Sequence      uint64             `json:"sequence"`
	ObservedAt    time.Time          `json:"observed_at"`
	AgentVersion  string             `json:"agent_version"`
	Collectors    map[string]Section `json:"collectors"`
}
type Ack struct {
	ID     string `json:"report_id"`
	Status string `json:"status"`
}
type Node struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Slug            string             `json:"slug"`
	Hostname        string             `json:"hostname"`
	Addresses       []string           `json:"addresses"`
	Kind            string             `json:"kind"`
	Lifecycle       string             `json:"lifecycle"`
	Purpose         string             `json:"purpose"`
	Criticality     string             `json:"criticality"`
	Labels          map[string]string  `json:"labels"`
	Tags            []string           `json:"tags"`
	Links           map[string]string  `json:"links"`
	CustomFields    map[string]any     `json:"custom_fields,omitempty"`
	Revision        int                `json:"revision"`
	Instance        string             `json:"agent_instance_id,omitempty"`
	Sequence        uint64             `json:"sequence"`
	LastReport      time.Time          `json:"last_report"`
	ReceivedAt      time.Time          `json:"received_at"`
	Observations    map[string]Section `json:"observations"`
	CollectorStatus map[string]Section `json:"collector_status"`
	Services        []Service          `json:"services"`
	Payment         *Payment           `json:"payment,omitempty"`
	Backups         []Backup           `json:"backups"`
}
type Service struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Purpose      string `json:"purpose"`
	Exposure     string `json:"exposure"`
	Primary      bool   `json:"primary"`
	Instructions string `json:"instructions"`
}
type Payment struct {
	NextDue  time.Time `json:"next_due"`
	Interval int       `json:"interval"`
	Unit     string    `json:"unit"`
	Timezone string    `json:"timezone"`
	Amount   string    `json:"amount"`
	Currency string    `json:"currency"`
	Note     string    `json:"note"`
	LastPaid time.Time `json:"last_paid"`
}
type Backup struct {
	ID                string    `json:"id"`
	Destination       string    `json:"destination"`
	Label             string    `json:"label"`
	IntervalHours     int       `json:"interval_hours"`
	GraceHours        int       `json:"grace_hours"`
	LastAttempt       time.Time `json:"last_attempt_at"`
	LastSuccess       time.Time `json:"last_success_at"`
	LastAttemptStatus string    `json:"last_attempt_status"`
	LastRestoreTest   time.Time `json:"last_restore_test_at"`
	ObservedAt        time.Time `json:"observed_at"`
	Evidence          string    `json:"evidence"`
}
type Relation struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	Target     string `json:"target"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	Provenance string `json:"provenance"`
	Revision   int    `json:"revision"`
}
type Document struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Kind              string    `json:"kind"`
	Body              string    `json:"body"`
	Nodes             []string  `json:"nodes"`
	Tags              []string  `json:"tags"`
	Revision          int       `json:"revision"`
	Author            string    `json:"author"`
	UpdatedAt         time.Time `json:"updated_at"`
	Status            string    `json:"status"`
	ApprovedRevision  int       `json:"approved_revision"`
	ApprovedHash      string    `json:"approved_hash,omitempty"`
	Approver          string    `json:"approver,omitempty"`
	EvidenceHash      string    `json:"evidence_hash,omitempty"`
	NeedsRevalidation bool      `json:"needs_revalidation"`
}
type Event struct {
	ID     string    `json:"id"`
	Kind   string    `json:"kind"`
	Entity string    `json:"entity"`
	Actor  string    `json:"actor"`
	At     time.Time `json:"at"`
	Detail string    `json:"detail"`
}
type Package struct {
	Name                   string `json:"name"`
	Arch                   string `json:"arch"`
	Version                string `json:"version"`
	Source                 string `json:"source"`
	SourceVersion          string `json:"source_version"`
	State                  string `json:"state"`
	Candidate              string `json:"candidate"`
	CandidateSource        string `json:"candidate_source"`
	CandidateSourceVersion string `json:"candidate_source_version"`
	Origin                 string `json:"origin"`
	OriginEvidence         string `json:"origin_evidence"`
	Suite                  string `json:"suite"`
	Held                   bool   `json:"held"`
	BlockedReason          string `json:"blocked_reason"`
}
type PackageData struct {
	Packages       []Package `json:"packages"`
	MetadataStatus string    `json:"metadata_status"`
	Simulation     string    `json:"simulation"`
}
type OSData struct {
	ID            string  `json:"id"`
	Codename      string  `json:"codename"`
	Version       string  `json:"version"`
	Hostname      string  `json:"hostname"`
	Arch          string  `json:"arch"`
	Kernel        string  `json:"kernel"`
	CPU           int     `json:"cpu"`
	MemoryBytes   uint64  `json:"memory_bytes"`
	UptimeSeconds float64 `json:"uptime_seconds"`
	Reboot        string  `json:"reboot"`
}
type Advisory struct {
	ID         string `json:"id"`
	NativeID   string `json:"native_id"`
	Provider   string `json:"provider"`
	Distro     string `json:"distro"`
	Release    string `json:"release"`
	Channel    string `json:"channel"`
	Source     string `json:"source"`
	Introduced string `json:"introduced"`
	Fixed      string `json:"fixed"`
	Status     string `json:"status"`
	Severity   string `json:"severity"`
	Notes      string `json:"notes"`
	URL        string `json:"url"`
	Withdrawn  bool   `json:"withdrawn"`
}
type Feed struct {
	ID             string    `json:"id"`
	LastAttempt    time.Time `json:"last_attempt"`
	LastSuccess    time.Time `json:"last_successful_sync"`
	SourceModified string    `json:"last_source_modified"`
	ETag           string    `json:"etag"`
	Error          string    `json:"error,omitempty"`
	Count          int       `json:"count"`
}
type Finding struct {
	ID              string    `json:"id"`
	NodeID          string    `json:"node_id"`
	Advisory        Advisory  `json:"advisory"`
	Packages        []string  `json:"packages"`
	Assessment      string    `json:"assessment"`
	Remediation     string    `json:"remediation"`
	Workflow        string    `json:"workflow"`
	Reason          string    `json:"reason"`
	SuppressedUntil time.Time `json:"suppressed_until"`
	FirstSeen       time.Time `json:"first_seen"`
	LastEvaluated   time.Time `json:"last_evaluated"`
	InventoryAt     time.Time `json:"inventory_at"`
	FeedAt          time.Time `json:"feed_at"`
}
type Attention struct {
	NodeID     string    `json:"node_id"`
	Name       string    `json:"name"`
	Reasons    []string  `json:"reasons"`
	LastReport time.Time `json:"last_report"`
}
