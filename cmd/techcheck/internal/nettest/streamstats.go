package nettest

import (
	"math"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// StreamPacket is one data packet of a UDP stream as the client received it.
// SendNS is the beacon's clock, RecvNS the client's.
type StreamPacket struct {
	Seq    uint32
	SendNS int64
	RecvNS int64
}

// StreamStats summarises the data packets of one stream, given in arrival
// order. totalSent is the count from the beacon's end packet; pass 0 when no
// end packet arrived, and the highest seq seen + 1 stands in for it.
//
// Duplicates count once. Reorder counts packets whose seq is below the highest
// seq already seen. Jitter is the RFC 3550 interarrival jitter over transit
// time (RecvNS - SendNS); the clock offset between beacon and client cancels
// out in the differences. Jitter is nil when fewer than 2 distinct packets
// arrived, and reorder is nil when none did: neither was measured.
func StreamStats(rateMbps float64, totalSent int, pkts []StreamPacket) report.StreamStep {
	seen := make(map[uint32]bool, len(pkts))
	var (
		received    int
		reordered   int
		maxSeq      uint32
		prevTransit int64
		jitterNS    float64
	)
	for _, p := range pkts {
		if seen[p.Seq] {
			continue
		}
		seen[p.Seq] = true
		transit := p.RecvNS - p.SendNS
		if received > 0 {
			if p.Seq < maxSeq {
				reordered++
			}
			d := float64(transit - prevTransit)
			jitterNS += (math.Abs(d) - jitterNS) / 16
		}
		if received == 0 || p.Seq > maxSeq {
			maxSeq = p.Seq
		}
		prevTransit = transit
		received++
	}

	total := totalSent
	if total <= 0 && received > 0 {
		total = int(maxSeq) + 1
	}
	step := report.StreamStep{
		Mbps:     rateMbps,
		Sent:     total,
		Received: received,
		LossPct:  100,
	}
	if total > 0 {
		lost := max(total-received, 0)
		step.LossPct = float64(lost) * 100 / float64(total)
	}
	if received > 0 {
		reorder := float64(reordered) * 100 / float64(received)
		step.ReorderPct = &reorder
	}
	if received > 1 {
		jitter := jitterNS / 1e6
		step.JitterMS = &jitter
	}
	return step
}
