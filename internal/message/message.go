package message

import (
	"fmt"
	"net"

	"golang.org/x/net/dns/dnsmessage"

	"zonedns/internal/model"
)

const ClassIN = dnsmessage.ClassINET

func BuildQuery(name string, rtype model.RecordType) ([]byte, error) {
	dnsName, err := dnsmessage.NewName(fqdn(name))
	if err != nil {
		return nil, fmt.Errorf("encode name %q: %w", name, err)
	}
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
	if err := builder.StartQuestions(); err != nil {
		return nil, fmt.Errorf("start questions: %w", err)
	}
	if err := builder.Question(dnsmessage.Question{
		Name:  dnsName,
		Type:  toWireType(rtype),
		Class: ClassIN,
	}); err != nil {
		return nil, fmt.Errorf("append question: %w", err)
	}
	return builder.Finish()
}

func ParseQuery(data []byte) (model.Query, error) {
	var parser dnsmessage.Parser
	header, err := parser.Start(data)
	if err != nil {
		return model.Query{}, fmt.Errorf("parse header: %w", err)
	}
	if header.Response {
		return model.Query{}, fmt.Errorf("unexpected response message")
	}
	question, err := parser.Question()
	if err != nil {
		return model.Query{}, fmt.Errorf("parse question: %w", err)
	}
	return model.Query{
		Name: trimDot(question.Name.String()),
		Type: fromWireType(question.Type),
	}, nil
}

func BuildAnswer(query model.Query, records []model.Record) ([]byte, error) {
	dnsName, err := dnsmessage.NewName(fqdn(query.Name))
	if err != nil {
		return nil, fmt.Errorf("encode answer name: %w", err)
	}
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true, RCode: dnsmessage.RCodeSuccess})
	builder.EnableCompression()
	if err := builder.StartQuestions(); err != nil {
		return nil, fmt.Errorf("start questions: %w", err)
	}
	if err := builder.Question(dnsmessage.Question{
		Name: dnsName, Type: toWireType(query.Type), Class: ClassIN,
	}); err != nil {
		return nil, fmt.Errorf("append question: %w", err)
	}
	if err := builder.StartAnswers(); err != nil {
		return nil, fmt.Errorf("start answers: %w", err)
	}
	for _, record := range records {
		if err := appendAnswer(&builder, record); err != nil {
			return nil, err
		}
	}
	return builder.Finish()
}

func BuildNXDomain(query model.Query, soa model.SOA) ([]byte, error) {
	dnsName, err := dnsmessage.NewName(fqdn(query.Name))
	if err != nil {
		return nil, fmt.Errorf("encode nxdomain name: %w", err)
	}
	soaName, err := dnsmessage.NewName(fqdn(soa.MName))
	if err != nil {
		return nil, fmt.Errorf("encode soa mname: %w", err)
	}
	mbox, err := dnsmessage.NewName(fqdn(soa.RName))
	if err != nil {
		return nil, fmt.Errorf("encode soa rname: %w", err)
	}
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true, RCode: dnsmessage.RCodeNameError})
	builder.EnableCompression()
	if err := builder.StartQuestions(); err != nil {
		return nil, fmt.Errorf("start questions: %w", err)
	}
	if err := builder.Question(dnsmessage.Question{
		Name: dnsName, Type: toWireType(query.Type), Class: ClassIN,
	}); err != nil {
		return nil, fmt.Errorf("append question: %w", err)
	}
	if err := builder.StartAuthorities(); err != nil {
		return nil, fmt.Errorf("start authorities: %w", err)
	}
	soaHeader := dnsmessage.ResourceHeader{
		Name: dnsName, Type: dnsmessage.TypeSOA, Class: ClassIN, TTL: soa.Minimum,
	}
	soaBody := dnsmessage.SOAResource{
		NS: soaName, MBox: mbox, Serial: soa.Serial,
		Refresh: soa.Refresh, Retry: soa.Retry, Expire: soa.Expire, MinTTL: soa.Minimum,
	}
	if err := builder.SOAResource(soaHeader, soaBody); err != nil {
		return nil, fmt.Errorf("append soa authority: %w", err)
	}
	return builder.Finish()
}

func BuildNotify(zone string, serial uint32) ([]byte, error) {
	dnsName, err := dnsmessage.NewName(fqdn(zone))
	if err != nil {
		return nil, fmt.Errorf("encode notify zone: %w", err)
	}
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{OpCode: dnsmessage.OpCode(4)})
	if err := builder.StartQuestions(); err != nil {
		return nil, fmt.Errorf("start notify question: %w", err)
	}
	if err := builder.Question(dnsmessage.Question{
		Name: dnsName, Type: dnsmessage.TypeSOA, Class: ClassIN,
	}); err != nil {
		return nil, fmt.Errorf("append notify question: %w", err)
	}
	if err := builder.StartAdditionals(); err != nil {
		return nil, fmt.Errorf("start additional: %w", err)
	}
	header := dnsmessage.ResourceHeader{Name: dnsName, Type: dnsmessage.TypeSOA, Class: ClassIN, TTL: 0}
	body := dnsmessage.SOAResource{
		NS: dnsName, MBox: dnsName, Serial: serial,
		Refresh: 0, Retry: 0, Expire: 0, MinTTL: 0,
	}
	if err := builder.SOAResource(header, body); err != nil {
		return nil, fmt.Errorf("append notify soa: %w", err)
	}
	return builder.Finish()
}

func ParseNotifySerial(data []byte) (string, uint32, error) {
	var parser dnsmessage.Parser
	if _, err := parser.Start(data); err != nil {
		return "", 0, fmt.Errorf("parse notify: %w", err)
	}
	question, err := parser.Question()
	if err != nil {
		return "", 0, fmt.Errorf("parse notify question: %w", err)
	}
	parser.SkipAllQuestions()
	parser.SkipAllAnswers()
	parser.SkipAllAuthorities()
	for {
		if _, err := parser.AdditionalHeader(); err != nil {
			if err == dnsmessage.ErrSectionDone {
				break
			}
			return "", 0, fmt.Errorf("parse notify additional header: %w", err)
		}
		resource, err := parser.Additional()
		if err == dnsmessage.ErrSectionDone {
			break
		}
		if err != nil {
			return "", 0, fmt.Errorf("parse notify additional: %w", err)
		}
		soa, ok := resource.Body.(*dnsmessage.SOAResource)
		if ok {
			return trimDot(question.Name.String()), soa.Serial, nil
		}
	}
	return trimDot(question.Name.String()), 0, nil
}

func appendAnswer(builder *dnsmessage.Builder, record model.Record) error {
	name, err := dnsmessage.NewName(fqdn(record.Name))
	if err != nil {
		return fmt.Errorf("encode record name %q: %w", record.Name, err)
	}
	header := dnsmessage.ResourceHeader{
		Name:  name,
		Class: ClassIN,
		TTL:   record.TTL,
	}
	switch record.Type {
	case model.TypeA:
		ip := net.ParseIP(record.RData).To4()
		if ip == nil {
			return fmt.Errorf("invalid A rdata %q", record.RData)
		}
		header.Type = dnsmessage.TypeA
		return builder.AResource(header, dnsmessage.AResource{A: [4]byte{ip[0], ip[1], ip[2], ip[3]}})
	case model.TypeAAAA:
		ip := net.ParseIP(record.RData).To16()
		if ip == nil {
			return fmt.Errorf("invalid AAAA rdata %q", record.RData)
		}
		header.Type = dnsmessage.TypeAAAA
		var body [16]byte
		copy(body[:], ip)
		return builder.AAAAResource(header, dnsmessage.AAAAResource{AAAA: body})
	case model.TypeTXT:
		header.Type = dnsmessage.TypeTXT
		return builder.TXTResource(header, dnsmessage.TXTResource{TXT: []string{record.RData}})
	case model.TypeCNAME:
		target, err := dnsmessage.NewName(fqdn(record.RData))
		if err != nil {
			return fmt.Errorf("encode cname target: %w", err)
		}
		header.Type = dnsmessage.TypeCNAME
		return builder.CNAMEResource(header, dnsmessage.CNAMEResource{CNAME: target})
	case model.TypeNS:
		target, err := dnsmessage.NewName(fqdn(record.RData))
		if err != nil {
			return fmt.Errorf("encode ns target: %w", err)
		}
		header.Type = dnsmessage.TypeNS
		return builder.NSResource(header, dnsmessage.NSResource{NS: target})
	default:
		return fmt.Errorf("unsupported answer record type %s", record.Type)
	}
}

func toWireType(rtype model.RecordType) dnsmessage.Type {
	switch rtype {
	case model.TypeA:
		return dnsmessage.TypeA
	case model.TypeAAAA:
		return dnsmessage.TypeAAAA
	case model.TypeCNAME:
		return dnsmessage.TypeCNAME
	case model.TypeMX:
		return dnsmessage.TypeMX
	case model.TypeTXT:
		return dnsmessage.TypeTXT
	case model.TypeNS:
		return dnsmessage.TypeNS
	case model.TypeSOA:
		return dnsmessage.TypeSOA
	default:
		return dnsmessage.Type(0)
	}
}

func fromWireType(wire dnsmessage.Type) model.RecordType {
	switch wire {
	case dnsmessage.TypeA:
		return model.TypeA
	case dnsmessage.TypeAAAA:
		return model.TypeAAAA
	case dnsmessage.TypeCNAME:
		return model.TypeCNAME
	case dnsmessage.TypeMX:
		return model.TypeMX
	case dnsmessage.TypeTXT:
		return model.TypeTXT
	case dnsmessage.TypeNS:
		return model.TypeNS
	case dnsmessage.TypeSOA:
		return model.TypeSOA
	default:
		return 0
	}
}

func trimDot(name string) string {
	if len(name) > 1 && name[len(name)-1] == '.' {
		return name[:len(name)-1]
	}
	return name
}

func fqdn(name string) string {
	if name == "" {
		return "."
	}
	if name[len(name)-1] != '.' {
		return name + "."
	}
	return name
}
