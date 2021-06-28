package csxegts

import (
	"encoding/binary"
	"math"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------------
// Counter
// ---------------------------------------------------------------------------------

// Counter is a struct for generating IDs for EGTS packets
type Counter struct {
	accumulator int32
}

func NewCounter() *Counter {
	return &Counter{accumulator: -1}
}

// Next returns next value of counter
//
// Based on getNextPid() & getNextRN() from github.com/kuznetsovin/egts-protocol
func (counter *Counter) Next() uint16 {
	if counter.accumulator < math.MaxUint16 {
		atomic.AddInt32(&counter.accumulator, 1)
	} else {
		counter.accumulator = 0
	}

	return uint16(atomic.LoadInt32(&counter.accumulator))
}

// ---------------------------------------------------------------------------------
// EGTS Time
// ---------------------------------------------------------------------------------

func EgtsTimeNowSeconds() uint32 {
	startDate := time.Date(2010, time.January, 1, 0, 0, 0, 0, time.UTC)
	return uint32(time.Since(startDate).Seconds())
}

// ---------------------------------------------------------------------------------
// EGTS Bytes
// ---------------------------------------------------------------------------------

func float64ToByteArr(f float64, n int) []byte {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(f))
	return buf[:n]
}