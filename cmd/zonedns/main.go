package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"zonedns/internal/cache"
	"zonedns/internal/control"
	"zonedns/internal/journal"
	"zonedns/internal/keyring"
	"zonedns/internal/message"
	"zonedns/internal/metric"
	"zonedns/internal/model"
	"zonedns/internal/record"
	"zonedns/internal/resolver"
	"zonedns/internal/signer"
	"zonedns/internal/transfer"
	"zonedns/internal/update"
	"zonedns/internal/zone"
)

func main() {
	httpAddr := flag.String("http", "", "HTTP listen address, e.g. 127.0.0.1:18080")
	demo := flag.Bool("demo", false, "run the scripted demo scenario and exit")
	journalDir := flag.String("journal-dir", "", "directory for the change journal")
	flag.Parse()

	app := buildApplication(*journalDir)
	if *demo {
		if err := runDemo(app); err != nil {
			log.Fatalf("demo failed: %v", err)
		}
		return
	}
	if *httpAddr == "" {
		flag.Usage()
		os.Exit(2)
	}
	server := app.httpserver(*httpAddr)
	sweepDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				app.cache.SweepExpired(now)
			case <-sweepDone:
				return
			}
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Printf("zonedns listening on %s", *httpAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http server error: %v", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	close(sweepDone)
	if err := app.journal.Close(); err != nil {
		log.Printf("close journal: %v", err)
	}
}

type application struct {
	zones     *zone.Store
	journal   *journal.Journal
	transfers *transfer.Service
	updates   *update.Service
	resolver  *resolver.Resolver
	control   *control.Service
	metrics   *metric.Metrics
	cache     *cache.Cache
	signer    *signer.Signer
	keys      *keyring.Keyring
	notifier  *transfer.MemoryNotifier
}

func buildApplication(journalDir string) *application {
	metrics := metric.New()
	keys := keyring.New()
	key1, err := keyring.Generate("k1")
	if err != nil {
		log.Fatalf("generate k1: %v", err)
	}
	if err := keys.Add(key1); err != nil {
		log.Fatalf("add k1: %v", err)
	}
	zones := zone.NewStore()
	signerService := signer.New(keys, "example.com")
	signerService.SetLookup(func(zoneName, name string, rtype model.RecordType) ([]model.Record, bool) {
		z, err := zones.Get(zoneName)
		if err != nil {
			return nil, false
		}
		return z.Lookup(name, rtype)
	})
	notifier := transfer.NewMemoryNotifier()
	transfers := transfer.New(zones, notifier)

	seedZone(zones, signerService, "example.com", 2026082201)
	seedZone(zones, signerService, "example.org", 2026082201)

	durability := journal.NewInMemory()
	if journalDir != "" {
		fileJournal, err := journal.New(journalDir)
		if err != nil {
			log.Fatalf("open journal: %v", err)
		}
		durability = fileJournal
	}
	responseCache := cache.New(2048)
	updates := update.New(zones, durability, transfers, metrics, responseCache)
	if err := journal.Replay(durability, updates); err != nil {
		log.Fatalf("replay journal: %v", err)
	}
	resolverService := resolver.New(zones, responseCache, signerService, metrics)
	controlService := control.New(zones, signerService, transfers, metrics)
	secondaries := zone.NewStore()
	for _, name := range zones.List() {
		z, _ := zones.Get(name)
		meta := z.Meta()
		meta.Primary = false
		secondaries.Put(zone.New(name, meta, record.New()))
	}
	transfers.SetSecondaries(secondaries)

	for _, name := range zones.List() {
		z, _ := zones.Get(name)
		z.SetResignHook(signerService.OnZoneUpdated)
		z.SetNotifyCallback(func(zoneName string, serial uint32) {
			metrics.IncNotify()
			if err := transfers.NotifyZone(zoneName, serial); err != nil {
				metrics.IncSignFailure()
			}
		})
	}
	return &application{
		zones:     zones,
		journal:   durability,
		transfers: transfers,
		updates:   updates,
		resolver:  resolverService,
		control:   controlService,
		metrics:   metrics,
		cache:     responseCache,
		signer:    signerService,
		keys:      keys,
		notifier:  notifier,
	}
}

func seedZone(zones *zone.Store, sig *signer.Signer, name string, serial uint32) {
	meta := model.ZoneMeta{
		Name:    name,
		Class:   "IN",
		Primary: true,
		SOA: model.SOA{
			MName:   "ns1." + name,
			RName:   "hostmaster." + name,
			Serial:  serial,
			Refresh: 3600,
			Retry:   600,
			Expire:  86400,
			Minimum: 300,
		},
	}
	records := record.New()
	z := zone.New(name, meta, records)
	seeds := []model.Record{
		{Name: name, Type: model.TypeNS, TTL: 3600, RData: "ns1." + name},
		{Name: "ns1." + name, Type: model.TypeA, TTL: 3600, RData: "127.0.0.1"},
		{Name: "www." + name, Type: model.TypeA, TTL: 300, RData: "192.0.2.10"},
		{Name: "www." + name, Type: model.TypeAAAA, TTL: 300, RData: "2001:db8::10"},
		{Name: "mail." + name, Type: model.TypeMX, TTL: 300, RData: "10 mail." + name},
		{Name: "txt." + name, Type: model.TypeTXT, TTL: 300, RData: "v=spf1 -all"},
	}
	for _, seed := range seeds {
		if _, err := records.Apply(model.Change{Kind: model.ChangeUpsert, Record: seed}); err != nil {
			log.Fatalf("seed %s: %v", name, err)
		}
	}
	zones.Put(z)
	if err := sig.ResignZone(zoneRRsets(z)); err != nil {
		log.Fatalf("seed sign %s: %v", name, err)
	}
}

func zoneRRsets(z *zone.Zone) []model.RRSet {
	var rrsets []model.RRSet
	for _, name := range z.Records().Names() {
		for _, rtype := range []model.RecordType{model.TypeA, model.TypeAAAA, model.TypeCNAME, model.TypeMX, model.TypeTXT, model.TypeNS} {
			if records, ok := z.Lookup(name, rtype); ok {
				rrsets = append(rrsets, model.RRSet{Name: name, Type: rtype, Records: records})
			}
		}
	}
	return rrsets
}

func (a *application) httpserver(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		rtype, ok := model.ParseType(r.URL.Query().Get("type"))
		if name == "" || !ok {
			httpError(w, http.StatusBadRequest, "name and type are required")
			return
		}
		a.metrics.IncQuery()
		answer, err := a.resolver.Resolve(model.Query{Name: name, Type: rtype})
		if err != nil {
			httpError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, answer)
	})
	mux.HandleFunc("/dns", func(w http.ResponseWriter, r *http.Request) {
		encoded := r.URL.Query().Get("q")
		packet, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			httpError(w, http.StatusBadRequest, "invalid base64 query")
			return
		}
		response, err := a.resolver.ResolveWire(packet)
		if err != nil {
			httpError(w, http.StatusNotFound, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(response)
	})
	mux.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		zoneName := r.URL.Query().Get("zone")
		name := r.URL.Query().Get("name")
		rtype, ok := model.ParseType(r.URL.Query().Get("type"))
		value := r.URL.Query().Get("value")
		if zoneName == "" || name == "" || !ok || value == "" {
			httpError(w, http.StatusBadRequest, "zone,name,type,value are required")
			return
		}
		ch := model.Change{Kind: model.ChangeUpsert, Record: model.Record{Name: name, Type: rtype, TTL: 300, RData: value}}
		if err := a.updates.Apply(zoneName, ch); err != nil {
			httpError(w, http.StatusConflict, err.Error())
			return
		}
		a.metrics.IncUpdate()
		z := a.zones.MustGet(zoneName)
		if z == nil {
			httpError(w, http.StatusNotFound, "zone missing after apply")
			return
		}
		writeJSON(w, map[string]string{"status": "applied", "zone": zoneName, "serial": fmt.Sprintf("%d", z.Serial())})
	})
	mux.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		if err := a.control.Reload(r.URL.Query().Get("zone")); err != nil {
			httpError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "reloaded"})
	})
	mux.HandleFunc("/transfer", func(w http.ResponseWriter, r *http.Request) {
		if _, err := a.control.TriggerTransfer(r.URL.Query().Get("zone")); err != nil {
			httpError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "transfer-complete"})
	})
	mux.HandleFunc("/zones", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, a.control.Status())
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, a.metrics.Snapshot())
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		key, err := a.keys.SigningKey()
		pem := ""
		if err == nil {
			if encoded, pemErr := key.PublicPEM(); pemErr == nil {
				pem = string(encoded)
			}
		}
		writeJSON(w, map[string]any{
			"active": a.keys.ActiveKeyID(),
			"verify": a.keys.VerifyKeyIDs(),
			"all":    a.keys.KeyIDs(),
			"public": pem,
		})
	})
	mux.HandleFunc("/journal", func(w http.ResponseWriter, _ *http.Request) {
		entries, err := a.journal.Replay()
		if err != nil {
			httpError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, map[string]any{"entries": len(entries)})
	})
	mux.HandleFunc("/cache", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"entries": a.cache.Len()})
	})
	return &http.Server{Addr: addr, Handler: mux}
}

func runDemo(a *application) error {
	a.resolver.Resolve(model.Query{Name: "www.example.com", Type: model.TypeA})
	ch := model.Change{Kind: model.ChangeUpsert, Record: model.Record{Name: "api.example.com", Type: model.TypeA, TTL: 300, RData: "192.0.2.55"}}
	if err := a.updates.Apply("example.com", ch); err != nil {
		return fmt.Errorf("dynamic update: %w", err)
	}
	packet, err := message.BuildQuery("www.example.com", model.TypeA)
	if err != nil {
		return fmt.Errorf("build wire query: %w", err)
	}
	if _, err := a.resolver.ResolveWire(packet); err != nil {
		return fmt.Errorf("resolve wire query: %w", err)
	}
	snapshot, err := a.transfers.AXFR("example.com")
	if err != nil {
		return fmt.Errorf("axfr: %w", err)
	}
	if err := a.transfers.ApplyAXFR("example.com", snapshot); err != nil {
		return fmt.Errorf("apply axfr to secondary: %w", err)
	}
	notifyZone, notifySerial, err := message.ParseNotifySerial(a.notifier.LastPacket())
	if err != nil {
		return fmt.Errorf("parse notify packet: %w", err)
	}
	ch2 := model.Change{Kind: model.ChangeUpsert, Record: model.Record{Name: "api2.example.com", Type: model.TypeA, TTL: 300, RData: "192.0.2.88"}}
	if err := a.updates.Apply("example.com", ch2); err != nil {
		return fmt.Errorf("second dynamic update: %w", err)
	}
	delta, err := a.transfers.IXFR("example.com", snapshot.Serial)
	if err != nil {
		return fmt.Errorf("ixfr: %w", err)
	}
	if err := a.transfers.ApplyIXFR("example.com", delta); err != nil {
		return fmt.Errorf("apply ixfr to secondary: %w", err)
	}
	secondary, err := a.transfers.Secondary("example.com")
	if err != nil {
		return fmt.Errorf("secondary lookup zone: %w", err)
	}
	secondaryRecords, ok := secondary.Lookup("api2.example.com", model.TypeA)
	if !ok || len(secondaryRecords) == 0 {
		return fmt.Errorf("secondary missed ixfr record")
	}
	key2, err := keyring.Generate("k2")
	if err != nil {
		return fmt.Errorf("generate k2: %w", err)
	}
	if err := a.keys.Rollover(key2, time.Now()); err != nil {
		return fmt.Errorf("rollover: %w", err)
	}
	if err := a.control.Reload("example.com"); err != nil {
		return fmt.Errorf("reload after rollover: %w", err)
	}
	answer, err := a.resolver.Resolve(model.Query{Name: "mail.example.com", Type: model.TypeMX})
	if err != nil {
		return fmt.Errorf("resolve after rollover: %w", err)
	}
	if _, err := a.control.TriggerTransfer("example.org"); err != nil {
		return fmt.Errorf("trigger transfer: %w", err)
	}
	fmt.Printf("demo ok: %s -> %s (serial %d, ixfr ops %d, notify %s@%d, secondary %s)\n", answer.Name, answer.RData, snapshot.Serial, len(delta.Ops), notifyZone, notifySerial, secondaryRecords[0].RData)
	fmt.Printf("metrics: %s\n", compactJSON(a.metrics.Snapshot()))
	return nil
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func httpError(w http.ResponseWriter, code int, message string) {
	http.Error(w, message, code)
}

func compactJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
