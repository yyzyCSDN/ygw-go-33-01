package record

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"zonedns/internal/model"
)

func ValidateRecord(r model.Record) error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("record name is empty")
	}
	if r.Type < model.TypeA || r.Type > model.TypeRRSIG {
		return fmt.Errorf("unknown record type %v", r.Type)
	}
	if r.Type == model.TypeSOA || r.Type == model.TypeRRSIG {
		return fmt.Errorf("type %s cannot be managed through dynamic records", r.Type)
	}
	switch r.Type {
	case model.TypeA:
		ip := net.ParseIP(r.RData)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("invalid A value %q", r.RData)
		}
	case model.TypeAAAA:
		ip := net.ParseIP(r.RData)
		if ip == nil || ip.To4() != nil {
			return fmt.Errorf("invalid AAAA value %q", r.RData)
		}
	case model.TypeCNAME, model.TypeNS:
		if strings.TrimSpace(r.RData) == "" {
			return fmt.Errorf("%s target is empty", r.Type)
		}
	case model.TypeMX:
		parts := strings.Fields(r.RData)
		if len(parts) != 2 {
			return fmt.Errorf("MX value must be '<priority> <host>', got %q", r.RData)
		}
		priority, err := strconv.Atoi(parts[0])
		if err != nil || priority < 0 || priority > 65535 {
			return fmt.Errorf("invalid MX priority in %q", r.RData)
		}
		if strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("MX host is empty")
		}
	case model.TypeTXT:
		if len(r.RData) == 0 || len(r.RData) > 255 {
			return fmt.Errorf("TXT value length must be 1..255")
		}
	default:
		return fmt.Errorf("unsupported record type %s", r.Type)
	}
	return nil
}
