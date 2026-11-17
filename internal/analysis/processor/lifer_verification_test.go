package processor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/birdnet-go/internal/classifier"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore"
	"github.com/tphakala/birdnet-go/internal/notification"
)

// fakeLiferRunner returns one result set per PredictModel call, in order, and
// records the window lengths it was given.
type fakeLiferRunner struct {
	spec    classifier.ModelSpec
	results [][]datastore.Results
	err     error
	calls   []int
}

func (f *fakeLiferRunner) ModelSpecFor(string) (classifier.ModelSpec, bool) { return f.spec, true }

func (f *fakeLiferRunner) PredictModel(_ context.Context, _ string, sample [][]float32) ([]datastore.Results, error) {
	f.calls = append(f.calls, len(sample[0]))
	if f.err != nil {
		return nil, f.err
	}
	i := len(f.calls) - 1
	if i >= len(f.results) {
		return nil, nil
	}
	return f.results[i], nil
}

func TestLiferWindows(t *testing.T) {
	t.Parallel()

	samples := func(n int) []float32 {
		s := make([]float32, n)
		for i := range s {
			s[i] = float32(i)
		}
		return s
	}

	t.Run("short audio is zero padded to one window", func(t *testing.T) {
		t.Parallel()
		w := liferWindows(samples(3), 5, 4)
		require.Len(t, w, 1)
		assert.Equal(t, []float32{0, 1, 2, 0, 0}, w[0])
	})

	t.Run("half window hop with an end-aligned last window", func(t *testing.T) {
		t.Parallel()
		w := liferWindows(samples(13), 4, 10)
		require.Len(t, w, 6) // starts 0,2,4,6,8 then 9
		assert.InDelta(t, 0, w[0][0], 0)
		assert.InDelta(t, 9, w[5][0], 0)
		for _, win := range w {
			assert.Len(t, win, 4)
		}
	})

	t.Run("capped windows keep the first and last", func(t *testing.T) {
		t.Parallel()
		w := liferWindows(samples(100), 10, 3)
		require.Len(t, w, 3)
		assert.InDelta(t, 0, w[0][0], 0)
		assert.InDelta(t, 90, w[2][0], 0)
	})
}

func TestScoreLiferAudio(t *testing.T) {
	t.Parallel()

	spec := classifier.ModelSpec{SampleRate: 1000, ClipLength: time.Second}
	pcm := make([]byte, 2*2500) // 2.5s of 16-bit mono at 1kHz -> 4 windows

	t.Run("confirms on a later window and stops early", func(t *testing.T) {
		t.Parallel()
		r := &fakeLiferRunner{spec: spec, results: [][]datastore.Results{
			{{Species: "Turdus merula_Common Blackbird", Confidence: 0.3}},
			{{Species: "Turdus merula_Common Blackbird", Confidence: 0.8}},
		}}
		verdict, best, err := scoreLiferAudio(t.Context(), r, "perch", spec, pcm, 1000, "Turdus merula", 0.5)
		require.NoError(t, err)
		assert.Equal(t, liferConfirmed, verdict)
		assert.InDelta(t, 0.8, best, 1e-6)
		assert.Len(t, r.calls, 2)
		assert.Equal(t, 1000, r.calls[0])
	})

	t.Run("rejects when the species stays below the threshold", func(t *testing.T) {
		t.Parallel()
		r := &fakeLiferRunner{spec: spec, results: [][]datastore.Results{
			{{Species: "Turdus merula_Common Blackbird", Confidence: 0.2}},
			{{Species: "Erithacus rubecula_European Robin", Confidence: 0.9}},
		}}
		verdict, best, err := scoreLiferAudio(t.Context(), r, "perch", spec, pcm, 1000, "Turdus merula", 0.5)
		require.NoError(t, err)
		assert.Equal(t, liferRejected, verdict)
		assert.InDelta(t, 0.2, best, 1e-6)
		assert.Len(t, r.calls, liferVerifyMaxWindows)
	})

	t.Run("resamples to the verifier rate", func(t *testing.T) {
		t.Parallel()
		r := &fakeLiferRunner{spec: spec}
		_, _, err := scoreLiferAudio(t.Context(), r, "perch", spec, make([]byte, 2*4000), 2000, "Turdus merula", 0.5)
		require.NoError(t, err)
		require.NotEmpty(t, r.calls)
		assert.Equal(t, 1000, r.calls[0])
	})

	t.Run("an inference error leaves the lifer unverified", func(t *testing.T) {
		t.Parallel()
		r := &fakeLiferRunner{spec: spec, err: assert.AnError}
		verdict, _, err := scoreLiferAudio(t.Context(), r, "perch", spec, pcm, 1000, "Turdus merula", 0.5)
		require.Error(t, err)
		assert.Equal(t, liferUnverified, verdict)
	})

	t.Run("a raw ultrasonic verifier is refused", func(t *testing.T) {
		t.Parallel()
		raw := classifier.ModelSpec{SampleRate: 1000, RawSampleRate: 256000, ClipLength: time.Second}
		r := &fakeLiferRunner{spec: raw}
		verdict, _, err := scoreLiferAudio(t.Context(), r, "bat", raw, pcm, 1000, "Turdus merula", 0.5)
		require.Error(t, err)
		assert.Equal(t, liferUnverified, verdict)
		assert.Empty(t, r.calls)
	})
}

// TestNotifyLiferRejected uses the process-wide notification service, so it does
// not run in parallel.
func TestNotifyLiferRejected(t *testing.T) {
	notification.ResetForTest()
	notification.Initialize(notification.DefaultServiceConfig())
	t.Cleanup(func() {
		notification.GetService().Stop()
		notification.ResetForTest()
	})

	det := testDetection()
	det.Result.ID = 42
	action := &DatabaseAction{Result: det.Result, CorrelationID: "test-lifer-rejected"}
	cfg := &conf.LiferVerificationSettings{Enabled: true, Model: "perch_v2", MinConfidence: 0.6, NotifyRejected: true}

	action.notifyLiferRejected(cfg, 0.12)

	list, err := notification.GetService().List(&notification.FilterOptions{})
	require.NoError(t, err)
	require.Len(t, list, 1)
	n := list[0]
	assert.Equal(t, notification.TypeDetection, n.Type)
	assert.Equal(t, notification.PriorityHigh, n.Priority)
	assert.Equal(t, "detection", n.Component)
	assert.Contains(t, n.Title, det.Result.Species.CommonName)
	assert.Contains(t, n.Message, "perch_v2 scored it 12%")
	assert.Contains(t, n.Message, "needs 60%")
	assert.Equal(t, uint(42), n.Metadata["note_id"])
}
