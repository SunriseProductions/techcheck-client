package nettest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
)

// seqPackets returns packets for seqs in the given arrival order. Each packet
// leaves 1 ms after the previous one and takes transit(seq) ns to arrive.
func seqPackets(seqs []uint32, transit func(seq uint32) int64) []nettest.StreamPacket {
	pkts := make([]nettest.StreamPacket, 0, len(seqs))
	for _, s := range seqs {
		send := int64(s) * 1_000_000
		pkts = append(pkts, nettest.StreamPacket{Seq: s, SendNS: send, RecvNS: send + transit(s)})
	}
	return pkts
}

func inOrder(n int) []uint32 {
	seqs := make([]uint32, n)
	for i := range seqs {
		seqs[i] = uint32(i)
	}
	return seqs
}

func constTransit(uint32) int64 { return 5_000_000 }

func TestStreamStats(t *testing.T) {
	missingTwo := inOrder(100)
	missingTwo = append(missingTwo[:10], missingTwo[11:]...) // drop seq 10
	missingTwo = append(missingTwo[:50], missingTwo[51:]...) // drop seq 51

	swapped := inOrder(100)
	swapped[50], swapped[51] = swapped[51], swapped[50]

	tests := []struct {
		name        string
		totalSent   int
		seqs        []uint32
		transit     func(uint32) int64
		wantSent    int
		wantRecv    int
		wantLoss    float64
		wantReorder float64
		wantJitter  float64
	}{
		{
			name: "no loss", totalSent: 100, seqs: inOrder(100), transit: constTransit,
			wantSent: 100, wantRecv: 100,
		},
		{
			name: "two of 100 missing", totalSent: 100, seqs: missingTwo, transit: constTransit,
			wantSent: 100, wantRecv: 98, wantLoss: 2.0,
		},
		{
			name: "one swapped pair", totalSent: 100, seqs: swapped, transit: constTransit,
			wantSent: 100, wantRecv: 100, wantReorder: 1.0,
		},
		{
			name: "constant transit has no jitter", totalSent: 10, seqs: inOrder(10), transit: constTransit,
			wantSent: 10, wantRecv: 10, wantJitter: 0,
		},
		{
			name: "missing end packet uses highest seq plus one", totalSent: 0,
			seqs: []uint32{0, 1, 2, 4}, transit: constTransit,
			wantSent: 5, wantRecv: 4, wantLoss: 20.0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nettest.StreamStats(10, tc.totalSent, seqPackets(tc.seqs, tc.transit))
			assert.Equal(t, 10.0, got.Mbps)
			assert.Equal(t, tc.wantSent, got.Sent)
			assert.Equal(t, tc.wantRecv, got.Received)
			assert.InDelta(t, tc.wantLoss, got.LossPct, 1e-9)
			assert.InDelta(t, tc.wantReorder, got.ReorderPct, 1e-9)
			assert.InDelta(t, tc.wantJitter, got.JitterMS, 1e-9)
		})
	}
}

func TestStreamStatsJitterFollowsRFC3550(t *testing.T) {
	// Transit alternates 5 ms and 7 ms, so every |D| is 2 ms. J starts at 0
	// and moves 1/16 of the way to 2 ms per packet.
	pkts := seqPackets(inOrder(3), func(seq uint32) int64 {
		if seq%2 == 0 {
			return 5_000_000
		}
		return 7_000_000
	})
	got := nettest.StreamStats(10, 3, pkts)
	j := 0.0
	for range 2 {
		j += (2.0 - j) / 16
	}
	assert.InDelta(t, j, got.JitterMS, 1e-9)
}

func TestStreamStatsNoPackets(t *testing.T) {
	got := nettest.StreamStats(5, 150, nil)
	assert.Equal(t, 150, got.Sent)
	assert.Equal(t, 0, got.Received)
	assert.InDelta(t, 100.0, got.LossPct, 1e-9)
}
