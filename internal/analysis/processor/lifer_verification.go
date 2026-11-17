package processor

import (
	"context"
	"fmt"
	"math"
	"time"

	dsppcm "github.com/tphakala/go-audio-dsp/pcm"

	"github.com/tphakala/birdnet-go/internal/audiocore/resample"
	"github.com/tphakala/birdnet-go/internal/classifier"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore"
	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/logger"
	"github.com/tphakala/birdnet-go/internal/notification"
	"github.com/tphakala/birdnet-go/internal/speciesindex"
)

// liferVerifyTimeout bounds the whole re-analysis of one lifer. It runs inside the
// Database step of the CompositeAction (CompositeActionTimeout, 10s), after which the
// lifer may still wait up to newSpeciesImageWait for its image, so the two together
// stay well inside the step budget.
const liferVerifyTimeout = 5 * time.Second

// liferVerifyPad widens the re-read audio on each side of the detection, so a call
// that straddles the detection's window edge is still whole in at least one of the
// verifier's windows.
const liferVerifyPad = time.Second

// liferVerifyMaxWindows caps how many verifier windows one lifer may score. A long
// Extended Capture span would otherwise cost one inference per window on hosts
// where a single Perch window can take a second or more.
const liferVerifyMaxWindows = 4

// liferVerdict is the outcome of re-analysing a lifer with the verifying model.
type liferVerdict int

const (
	// liferUnverified means the check could not run; the lifer alert is sent anyway.
	liferUnverified liferVerdict = iota
	// liferConfirmed means the verifier reported the species at or above the threshold.
	liferConfirmed
	// liferRejected means the verifier scored the audio and did not confirm the species.
	liferRejected
)

func (v liferVerdict) String() string {
	switch v {
	case liferConfirmed:
		return "confirmed"
	case liferRejected:
		return "rejected"
	default:
		return "unverified"
	}
}

// liferModelRunner is the part of the classifier orchestrator the verifier uses.
type liferModelRunner interface {
	ModelSpecFor(modelID string) (classifier.ModelSpec, bool)
	PredictModel(ctx context.Context, modelID string, sample [][]float32) ([]datastore.Results, error)
}

// verifyLifer re-analyses this detection's audio with the configured verifying
// model and reports whether the lifer alert should still be sent: true unless the
// verifier ran and rejected the species. Called only for a detection that
// shouldNotifyLifer already accepted, so it costs inference only for lifers.
func (a *DatabaseAction) verifyLifer(ctx context.Context) bool {
	cfg := &a.Settings.Realtime.LifeList.Verification
	if !cfg.Enabled {
		return true
	}

	verdict, best, reason := a.runLiferVerification(ctx, cfg)

	fields := []logger.Field{
		logger.String("component", "analysis.processor.lifer_verification"),
		logger.String("detection_id", a.CorrelationID),
		logger.String("species", a.Result.Species.CommonName),
		logger.String("scientific_name", a.Result.Species.ScientificName),
		logger.Float64("confidence", a.Result.Confidence),
		logger.String("verifier", cfg.Model),
		logger.Float64("verifier_confidence", best),
		logger.Float64("min_confidence", cfg.EffectiveMinConfidence()),
		logger.String("verdict", verdict.String()),
		logger.String("operation", "lifer_verification"),
	}
	if reason != "" {
		fields = append(fields, logger.String("reason", reason))
	}
	if verdict == liferUnverified {
		GetLogger().Warn("Lifer could not be verified; sending the alert unverified", fields...)
	} else {
		GetLogger().Info("Lifer re-analysed by the verifying model", fields...)
	}
	if verdict == liferRejected && cfg.NotifyRejected {
		a.notifyLiferRejected(cfg, best)
	}
	return verdict != liferRejected
}

// notifyLiferRejected tells the user a lifer was withheld because the verifier
// did not confirm it. It is a TypeDetection notification in the "detection"
// component at PriorityHigh, like the lifer alert itself, so push providers that
// deliver lifers deliver it too, and it carries note_id so it links to the saved
// detection for a listen.
func (a *DatabaseAction) notifyLiferRejected(cfg *conf.LiferVerificationSettings, best float64) {
	if !notification.IsInitialized() {
		return
	}
	service := notification.GetService()
	if service == nil {
		return
	}

	title := "Lifer not confirmed: " + a.Result.Species.CommonName
	message := fmt.Sprintf("Detected by %s at %.0f%%, but %s scored it %.0f%% (needs %.0f%%). Kept as an ordinary detection.",
		a.Result.Model.Name, a.Result.Confidence*100, cfg.Model, best*100, cfg.EffectiveMinConfidence()*100)

	notif := notification.NewNotification(notification.TypeDetection, notification.PriorityHigh, title, message).
		WithComponent("detection").
		WithMetadata("species", a.Result.Species.CommonName).
		WithMetadata("scientific_name", a.Result.Species.ScientificName).
		WithMetadata("lifer_verification", liferRejected.String())
	if a.Result.ID != 0 {
		notif = notif.WithMetadata("note_id", a.Result.ID)
	}
	if err := service.CreateWithMetadata(notif); err != nil {
		GetLogger().Warn("Failed to send the lifer rejection notification",
			logger.String("component", "analysis.processor.lifer_verification"),
			logger.String("detection_id", a.CorrelationID),
			logger.Error(err))
	}
}

// runLiferVerification resolves the verifier, reads the detection's audio and
// scores it. reason explains an unverified verdict.
func (a *DatabaseAction) runLiferVerification(ctx context.Context, cfg *conf.LiferVerificationSettings) (verdict liferVerdict, best float64, reason string) {
	if a.processor == nil || a.processor.Bn == nil {
		return liferUnverified, 0, "classifier not available"
	}
	registryID, ok := classifier.ResolveConfigModelID(cfg.Model)
	if !ok {
		return liferUnverified, 0, "unknown verifier model"
	}
	info := classifier.ModelRegistry[registryID]
	if info.DetectionName == a.Result.Model.Name && info.DetectionVersion == a.Result.Model.Version {
		return liferUnverified, 0, "verifier is the model that made the detection"
	}
	spec, ok := a.processor.Bn.ModelSpecFor(registryID)
	if !ok {
		return liferUnverified, 0, "verifier model is not loaded; add it to models.enabled"
	}

	start := a.Result.BeginTime.Add(-liferVerifyPad)
	end := a.Result.EndTime.Add(liferVerifyPad)
	if minEnd := start.Add(spec.ClipLength); end.Before(minEnd) {
		end = minEnd
	}
	seconds := int(math.Ceil(end.Sub(start).Seconds()))
	pcm, err := a.processor.readCaptureSegment(a.Result.AudioSource.ID, start, seconds)
	if err != nil {
		return liferUnverified, 0, "audio no longer in the capture buffer: " + err.Error()
	}

	srcRate := a.Result.AudioSource.SampleRate
	if srcRate == 0 {
		srcRate = conf.SampleRate
	}

	vctx, cancel := context.WithTimeout(ctx, liferVerifyTimeout)
	defer cancel()
	verdict, best, err = scoreLiferAudio(vctx, a.processor.Bn, registryID, spec, pcm, srcRate,
		a.Result.Species.ScientificName, cfg.EffectiveMinConfidence())
	if err != nil {
		return liferUnverified, best, err.Error()
	}
	return verdict, best, ""
}

// scoreLiferAudio resamples 16-bit mono PCM at srcRate to the verifier's rate,
// scores up to liferVerifyMaxWindows windows of it, and confirms the lifer when any
// window reports scientificName at minConfidence or above. Species are compared by
// speciesindex.CanonicalKey, which applies the taxonomic alias map, so a species
// the two models name differently after a split or rename still matches.
func scoreLiferAudio(ctx context.Context, runner liferModelRunner, registryID string, spec classifier.ModelSpec, pcm []byte, srcRate int, scientificName string, minConfidence float64) (liferVerdict, float64, error) {
	if spec.RawSampleRate > 0 {
		return liferUnverified, 0, errors.Newf("verifier %s needs raw ultrasonic audio", registryID).
			Category(errors.CategoryValidation).Build()
	}
	rate := spec.EffectiveSampleRate()
	resampled, err := resample.ResampleBytes(pcm, srcRate, rate)
	if err != nil {
		return liferUnverified, 0, err
	}
	samples := make([]float32, len(resampled)/conf.BytesPerSample)
	if _, err := dsppcm.BytesToFloat32(samples, resampled[:len(samples)*conf.BytesPerSample]); err != nil {
		return liferUnverified, 0, err
	}

	target := speciesindex.CanonicalKey(scientificName)
	var best float64
	for _, window := range liferWindows(samples, spec.ClipSampleCount(), liferVerifyMaxWindows) {
		if err := ctx.Err(); err != nil {
			return liferUnverified, best, err
		}
		results, err := runner.PredictModel(ctx, registryID, [][]float32{window})
		if err != nil {
			return liferUnverified, best, err
		}
		for i := range results {
			if speciesindex.CanonicalKey(results[i].Species) != target {
				continue
			}
			best = max(best, float64(results[i].Confidence))
		}
		if best >= minConfidence {
			return liferConfirmed, best, nil
		}
	}
	return liferRejected, best, nil
}

// liferWindows splits samples into windows of clipLen with a half-window hop,
// always including one aligned to the end, and keeps at most maxWindows of them,
// evenly spaced. Audio shorter than one window is zero-padded to a single window.
func liferWindows(samples []float32, clipLen, maxWindows int) [][]float32 {
	if clipLen <= 0 || maxWindows <= 0 {
		return nil
	}
	if len(samples) <= clipLen {
		w := make([]float32, clipLen)
		copy(w, samples)
		return [][]float32{w}
	}

	hop := max(clipLen/2, 1)
	last := len(samples) - clipLen
	var starts []int
	for s := 0; s < last; s += hop {
		starts = append(starts, s)
	}
	starts = append(starts, last)

	if len(starts) > maxWindows {
		picked := make([]int, maxWindows)
		for i := range picked {
			picked[i] = starts[i*(len(starts)-1)/max(maxWindows-1, 1)]
		}
		starts = picked
	}

	windows := make([][]float32, len(starts))
	for i, s := range starts {
		windows[i] = samples[s : s+clipLen]
	}
	return windows
}
