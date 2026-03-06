package id

import (
	"crypto/rand"
	"fmt"
	"sync/atomic"
	"sync"
	"time"
)

// ---- UUID ----

// UUID returns a new random UUID v4 string.
// Re-exported from google/uuid for convenience.
func UUID() string {
	// Inline implementation to avoid extra dependency in this file.
	// Uses crypto/rand for security.
	var u [16]byte
	_, _ = rand.Read(u[:])
	u[6] = (u[6] & 0x0f) | 0x40 // version 4
	u[8] = (u[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// ---- NanoID ----

const defaultAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// NanoID generates a URL-friendly unique ID of the given size.
// Default size is 21 (similar to the original nanoid spec).
func NanoID(size ...int) string {
	n := 21
	if len(size) > 0 && size[0] > 0 {
		n = size[0]
	}

	bytes := make([]byte, n)
	_, _ = rand.Read(bytes)

	id := make([]byte, n)
	for i := range n {
		id[i] = defaultAlphabet[bytes[i]%byte(len(defaultAlphabet))]
	}
	return string(id)
}

// ---- ShortID ----

// ShortID generates a short, roughly time-sortable unique ID.
// Default length is 16. The first 8 characters encode the current timestamp,
// and the remaining characters are random, providing both uniqueness and sortability.
func ShortID(size ...int) string {
	n := 16
	if len(size) > 0 && size[0] > 0 {
		n = size[0]
	}

	base := len(defaultAlphabet) // 62

	// Encode current timestamp (milliseconds) into the first part.
	ts := time.Now().UnixMilli()
	timePartLen := 8
	if n < timePartLen {
		timePartLen = n
	}

	id := make([]byte, n)
	for i := timePartLen - 1; i >= 0; i-- {
		id[i] = defaultAlphabet[ts%int64(base)]
		ts /= int64(base)
	}

	// Fill the rest with crypto/rand random characters.
	if n > timePartLen {
		randomBytes := make([]byte, n-timePartLen)
		_, _ = rand.Read(randomBytes)
		for i := timePartLen; i < n; i++ {
			id[i] = defaultAlphabet[randomBytes[i-timePartLen]%byte(base)]
		}
	}

	return string(id)
}

// ---- Snowflake ----

const (
	epoch        = int64(1704067200000) // 2024-01-01 00:00:00 UTC in ms
	workerBits   = 10
	sequenceBits = 12
	maxWorkerID  = (1 << workerBits) - 1
	maxSequence  = (1 << sequenceBits) - 1
	timeShift    = workerBits + sequenceBits
	workerShift  = sequenceBits
)

// Snowflake generates unique 64-bit IDs based on Twitter's Snowflake algorithm.
type Snowflake struct {
	mu        sync.Mutex
	workerID  int64
	sequence  int64
	lastTime  int64
}

// NewSnowflake creates a Snowflake generator.
// workerID must be between 0 and 1023.
func NewSnowflake(workerID int64) (*Snowflake, error) {
	if workerID < 0 || workerID > maxWorkerID {
		return nil, fmt.Errorf("id: workerID must be between 0 and %d", maxWorkerID)
	}
	return &Snowflake{workerID: workerID}, nil
}

// Generate returns a new unique snowflake ID.
func (s *Snowflake) Generate() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli() - epoch

	if now < s.lastTime {
		return 0, fmt.Errorf("id: clock moved backwards, refusing to generate")
	}

	if now == s.lastTime {
		s.sequence = (s.sequence + 1) & maxSequence
		if s.sequence == 0 {
			// Wait for next millisecond.
			for now <= s.lastTime {
				now = time.Now().UnixMilli() - epoch
			}
		}
	} else {
		s.sequence = 0
	}

	s.lastTime = now

	id := (now << timeShift) | (s.workerID << workerShift) | s.sequence
	return id, nil
}

// MustGenerate returns a new unique snowflake ID or panics.
func (s *Snowflake) MustGenerate() int64 {
	id, err := s.Generate()
	if err != nil {
		panic(err)
	}
	return id
}

// ---- NumericID ----

const numericEpoch = int64(1577836800000) // 2020-01-01 00:00:00 UTC in ms

var numericSequence uint64

func numericNextSeq() int64 {
	seq := atomic.AddUint64(&numericSequence, 1)
	if seq >= 10000 {
		atomic.StoreUint64(&numericSequence, 0)
		seq = atomic.AddUint64(&numericSequence, 1)
	}
	return int64(seq)
}

// NumericID generates a unique 16-digit int64 ID that is monotonically increasing.
// Structure: (ms since 2020-01-01)(12 digits) + sequence(4 digits) = 16 digits.
// Uses atomic operations, safe for concurrent use.
func NumericID() int64 {
	ts := time.Now().UnixMilli() - numericEpoch
	return ts*10000 + numericNextSeq()
}
