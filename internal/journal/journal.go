package journal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Entry struct {
	Zone   string `json:"zone"`
	Serial uint32 `json:"serial"`
	Change Change `json:"change"`
}

type Change struct {
	Kind   string `json:"kind"`
	Record Record `json:"record"`
}

type Record struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	TTL   uint32 `json:"ttl"`
	RData string `json:"rdata"`
}

type Durability interface {
	Append(entry Entry) error
	Replay() ([]Entry, error)
}

type Journal struct {
	mu   sync.Mutex
	dir  string
	file *os.File
	fail func(entry Entry) error
}

func New(dir string) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create journal dir: %w", err)
	}
	path := filepath.Join(dir, "changes.journal")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}
	return &Journal{dir: dir, file: file}, nil
}

func NewInMemory() *Journal {
	return &Journal{}
}

func (j *Journal) SetFail(fail func(entry Entry) error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.fail = fail
}

func (j *Journal) Append(entry Entry) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.fail != nil {
		if err := j.fail(entry); err != nil {
			return err
		}
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode journal entry: %w", err)
	}
	if j.file == nil {
		return nil
	}
	if _, err := j.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write journal: %w", err)
	}
	if err := j.file.Sync(); err != nil {
		return fmt.Errorf("sync journal: %w", err)
	}
	return nil
}

func (j *Journal) Replay() ([]Entry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.file == nil {
		return nil, nil
	}
	path := filepath.Join(j.dir, "changes.journal")
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open journal for replay: %w", err)
	}
	defer file.Close()
	var entries []Entry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("decode journal entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan journal: %w", err)
	}
	return entries, nil
}

func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.file != nil {
		err := j.file.Close()
		j.file = nil
		return err
	}
	return nil
}
