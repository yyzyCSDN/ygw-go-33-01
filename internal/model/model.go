package model

import "strings"

type RecordType int

const (
	TypeA RecordType = iota + 1
	TypeAAAA
	TypeCNAME
	TypeMX
	TypeTXT
	TypeNS
	TypeSOA
	TypeRRSIG
)

func (t RecordType) String() string {
	switch t {
	case TypeA:
		return "A"
	case TypeAAAA:
		return "AAAA"
	case TypeCNAME:
		return "CNAME"
	case TypeMX:
		return "MX"
	case TypeTXT:
		return "TXT"
	case TypeNS:
		return "NS"
	case TypeSOA:
		return "SOA"
	case TypeRRSIG:
		return "RRSIG"
	default:
		return "UNKNOWN"
	}
}

func ParseType(text string) (RecordType, bool) {
	switch strings.ToUpper(strings.TrimSpace(text)) {
	case "A":
		return TypeA, true
	case "AAAA":
		return TypeAAAA, true
	case "CNAME":
		return TypeCNAME, true
	case "MX":
		return TypeMX, true
	case "TXT":
		return TypeTXT, true
	case "NS":
		return TypeNS, true
	case "SOA":
		return TypeSOA, true
	case "RRSIG":
		return TypeRRSIG, true
	default:
		return 0, false
	}
}

type Record struct {
	Name  string     `json:"name"`
	Type  RecordType `json:"type"`
	TTL   uint32     `json:"ttl"`
	RData string     `json:"rdata"`
}

type RRSet struct {
	Name    string
	Type    RecordType
	Records []Record
}

type SOA struct {
	MName   string `json:"mname"`
	RName   string `json:"rname"`
	Serial  uint32 `json:"serial"`
	Refresh uint32 `json:"refresh"`
	Retry   uint32 `json:"retry"`
	Expire  uint32 `json:"expire"`
	Minimum uint32 `json:"minimum"`
}

type ZoneMeta struct {
	Name    string `json:"name"`
	Class   string `json:"class"`
	SOA     SOA    `json:"soa"`
	Primary bool   `json:"primary"`
}

type ChangeKind int

const (
	ChangeUpsert ChangeKind = iota + 1
	ChangeDelete
)

func (k ChangeKind) String() string {
	switch k {
	case ChangeUpsert:
		return "upsert"
	case ChangeDelete:
		return "delete"
	default:
		return "unknown"
	}
}

type Change struct {
	Kind   ChangeKind
	Record Record
}

type SerialRange struct {
	Start uint32 `json:"start"`
	End   uint32 `json:"end"`
}

type DeltaOp struct {
	Kind   ChangeKind
	Record Record
}

type Delta struct {
	Zone  string      `json:"zone"`
	Range SerialRange `json:"range"`
	Ops   []DeltaOp   `json:"ops"`
}

type ZoneSnapshot struct {
	Name    string
	Meta    ZoneMeta
	Serial  uint32
	Records []Record
}

type Query struct {
	Name string
	Type RecordType
}

type Answer struct {
	Name     string
	Type     RecordType
	TTL      uint32
	RData    string
	NXDomain bool
}

type NotifyEvent struct {
	Zone   string
	Serial uint32
}

type TransferState int

const (
	TransferIdle TransferState = iota
	TransferInProgress
	TransferComplete
	TransferFailed
)

func (s TransferState) String() string {
	switch s {
	case TransferIdle:
		return "idle"
	case TransferInProgress:
		return "in-progress"
	case TransferComplete:
		return "complete"
	case TransferFailed:
		return "failed"
	default:
		return "unknown"
	}
}
