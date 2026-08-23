package journal

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"btc5m-execution-engine/internal/event"
)

const GenesisHash = "GENESIS"

type Envelope struct {
	JournalVersion string       `json:"journal_version"`
	Sequence       uint64       `json:"sequence"`
	PreviousHash   string       `json:"previous_hash"`
	RecordHash     string       `json:"record_hash"`
	Event          event.Record `json:"event"`
}

type hashMaterial struct {
	JournalVersion string       `json:"journal_version"`
	Sequence       uint64       `json:"sequence"`
	PreviousHash   string       `json:"previous_hash"`
	Event          event.Record `json:"event"`
}

func computeHash(version string, seq uint64, prev string, e event.Record) (string, error) {
	b, err := json.Marshal(hashMaterial{JournalVersion: version, Sequence: seq, PreviousHash: prev, Event: e})
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

type Writer struct {
	mu      sync.Mutex
	f       *os.File
	buf     *bufio.Writer
	version string
	seq     uint64
	last    string
}

func NewWriter(path string) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	return &Writer{f: f, buf: bufio.NewWriterSize(f, 1<<20), version: "JOURNAL_V1", last: GenesisHash}, nil
}

func (w *Writer) Append(e event.Record) (Envelope, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seq++
	h, err := computeHash(w.version, w.seq, w.last, e)
	if err != nil {
		return Envelope{}, err
	}
	env := Envelope{JournalVersion: w.version, Sequence: w.seq, PreviousHash: w.last, RecordHash: h, Event: e}
	b, err := json.Marshal(env)
	if err != nil {
		return Envelope{}, err
	}
	if _, err := w.buf.Write(b); err != nil {
		return Envelope{}, err
	}
	if err := w.buf.WriteByte('\n'); err != nil {
		return Envelope{}, err
	}
	w.last = h
	return env, nil
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	var errs []error
	if err := w.buf.Flush(); err != nil {
		errs = append(errs, err)
	}
	if err := w.f.Sync(); err != nil {
		errs = append(errs, err)
	}
	if err := w.f.Close(); err != nil {
		errs = append(errs, err)
	}
	w.f = nil
	return errors.Join(errs...)
}

type ValidationResult struct {
	Records      uint64 `json:"records"`
	TerminalHash string `json:"terminal_hash"`
}

func ValidateFile(path string) (ValidationResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return ValidationResult{}, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	s.Buffer(buf, 16*1024*1024)
	var seq uint64
	prev := GenesisHash
	line := 0
	for s.Scan() {
		line++
		var env Envelope
		if err := json.Unmarshal(s.Bytes(), &env); err != nil {
			return ValidationResult{}, fmt.Errorf("line %d decode: %w", line, err)
		}
		seq++
		if env.Sequence != seq {
			return ValidationResult{}, fmt.Errorf("line %d sequence: got %d want %d", line, env.Sequence, seq)
		}
		if env.PreviousHash != prev {
			return ValidationResult{}, fmt.Errorf("line %d previous_hash mismatch", line)
		}
		h, err := computeHash(env.JournalVersion, env.Sequence, env.PreviousHash, env.Event)
		if err != nil {
			return ValidationResult{}, err
		}
		if env.RecordHash != h {
			return ValidationResult{}, fmt.Errorf("line %d record_hash mismatch", line)
		}
		prev = env.RecordHash
	}
	if err := s.Err(); err != nil {
		return ValidationResult{}, err
	}
	return ValidationResult{Records: seq, TerminalHash: prev}, nil
}
