package nettest_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

func streamInput(srv *testStreamServer) nettest.StreamInput {
	return nettest.StreamInput{
		Host:        srv.host,
		Port:        srv.port,
		Magic:       "PFLT",
		RateMbps:    2,
		PacketBytes: 1250,
		Duration:    300 * time.Millisecond,
		Timeout:     time.Second,
	}
}

func TestStreamAgainstNormalServer(t *testing.T) {
	srv := streamServer(t, streamNormal)

	step, anyData, err := nettest.MeasureUDPStream(context.Background(), streamInput(srv))
	require.NoError(t, err)
	assert.True(t, anyData)
	assert.Equal(t, 2.0, step.Mbps)
	assert.Equal(t, 60, step.Sent) // 200 packets/s for 300 ms
	assert.Greater(t, step.Received, 0)
	assert.Equal(t, 0.0, step.LossPct)
}

func TestStreamAgainstEchoOnlyServer(t *testing.T) {
	srv := streamServer(t, streamEchoOnly)
	in := streamInput(srv)

	start := time.Now()
	_, _, err := nettest.MeasureUDPStream(context.Background(), in)
	assert.ErrorIs(t, err, nettest.ErrStreamUnsupported)
	assert.Less(t, time.Since(start), in.Timeout)
}

func TestStreamUnsupportedWhenNoCookie(t *testing.T) {
	// A listener that never answers.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()
	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, err := pc.ReadFrom(buf); err != nil {
				return
			}
		}
	}()
	host, port := hostPort(t, pc.LocalAddr().String())

	_, _, err = nettest.MeasureUDPStream(context.Background(), nettest.StreamInput{
		Host: host, Port: port, Magic: "PFLT", RateMbps: 2, PacketBytes: 1250,
		Duration: 300 * time.Millisecond, Timeout: 300 * time.Millisecond,
	})
	assert.ErrorIs(t, err, nettest.ErrStreamUnsupported)
}

func TestStreamBusyGivesUpAfterRetries(t *testing.T) {
	srv := streamServer(t, streamBusy)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, err := nettest.MeasureUDPStream(ctx, streamInput(srv))
	require.Error(t, err)
	assert.NotErrorIs(t, err, nettest.ErrStreamUnsupported)
	assert.Contains(t, err.Error(), "busy")
	assert.Equal(t, int32(4), srv.starts.Load())
}

func TestStreamBusyAfterStartRetriesThenSucceeds(t *testing.T) {
	srv := streamServer(t, streamBusyOnce)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	step, anyData, err := nettest.MeasureUDPStream(ctx, streamInput(srv))
	require.NoError(t, err)
	assert.True(t, anyData)
	assert.Equal(t, 60, step.Sent)
	assert.Equal(t, int32(2), srv.starts.Load())
}

func TestStreamResendsLostHello(t *testing.T) {
	srv := streamServer(t, streamDropFirstHello)
	in := streamInput(srv)
	in.Timeout = 600 * time.Millisecond

	step, anyData, err := nettest.MeasureUDPStream(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, anyData)
	assert.Greater(t, step.Received, 0)
	assert.Equal(t, int32(2), srv.hellos.Load())
}

func TestStreamErrorsWhenNothingFollowsStart(t *testing.T) {
	srv := streamServer(t, streamSilentAfterStart)

	_, _, err := nettest.MeasureUDPStream(context.Background(), streamInput(srv))
	require.Error(t, err)
	assert.NotErrorIs(t, err, nettest.ErrStreamUnsupported)
	// One retry with a fresh handshake, then give up.
	assert.Equal(t, int32(2), srv.starts.Load())
}

func TestLadderStopsAtFirstLossyStep(t *testing.T) {
	srv := streamServer(t, streamLossyAbove10)

	// The limit leaves headroom for a stray loopback drop on a busy runner;
	// the 20 Mbps step loses 5 %.
	ladder, err := nettest.RunLadder(context.Background(), streamInput(srv),
		[]float64{5, 10, 20, 35}, 300*time.Millisecond, 1.0)
	require.NoError(t, err)
	require.Len(t, ladder.Steps, 3)
	assert.Equal(t, []float64{5, 10, 20},
		[]float64{ladder.Steps[0].Mbps, ladder.Steps[1].Mbps, ladder.Steps[2].Mbps})
	assert.InDelta(t, 5.0, ladder.Steps[2].LossPct, 0.5)
	assert.Equal(t, 10.0, ladder.MaxCleanMbps)
	assert.Equal(t, report.LargePacketsOK, ladder.LargePackets)
	assert.Equal(t, srv.port, ladder.Port)
}

func TestLadderLargePacketsDropped(t *testing.T) {
	srv := streamServer(t, streamNoData)

	ladder, err := nettest.RunLadder(context.Background(), streamInput(srv),
		[]float64{5, 10, 20, 35}, 300*time.Millisecond, 0.1)
	require.NoError(t, err)
	assert.Equal(t, report.LargePacketsDropped, ladder.LargePackets)
	assert.Equal(t, 0.0, ladder.MaxCleanMbps)
	require.Len(t, ladder.Steps, 1)
	assert.Equal(t, 150, ladder.Steps[0].Sent) // from E: 500 packets/s for 300 ms
	assert.Equal(t, 0, ladder.Steps[0].Received)
	assert.Equal(t, 100.0, ladder.Steps[0].LossPct)
}

func TestLadderReturnsErrorWhenStepDoesNotRun(t *testing.T) {
	srv := streamServer(t, streamSilentAfterStart)

	ladder, err := nettest.RunLadder(context.Background(), streamInput(srv),
		[]float64{5, 10}, 300*time.Millisecond, 0.1)
	require.Error(t, err)
	assert.Empty(t, ladder.Steps)
	assert.Empty(t, ladder.LargePackets)
}

func TestLadderReturnsUnsupportedError(t *testing.T) {
	srv := streamServer(t, streamEchoOnly)

	ladder, err := nettest.RunLadder(context.Background(), streamInput(srv),
		[]float64{5, 10}, 300*time.Millisecond, 0.1)
	assert.ErrorIs(t, err, nettest.ErrStreamUnsupported)
	assert.Empty(t, ladder.Steps)
}
