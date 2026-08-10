package dispatch

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type etc struct {
	concurrency     int
	minimumDuration time.Duration
	successes       []time.Duration
	failures        []time.Duration
	mutex           *sync.RWMutex
}

func FriendlyDuration(d time.Duration) string {
	// A negative duration means an estimate overshot; report it as zero rather
	// than showing the user something like "-58 milliseconds".
	if d < 0 {
		d = 0
	}
	if d < 2*time.Second {
		return fmt.Sprintf("%.0f milliseconds", d.Seconds()*1000)
	}
	if d < time.Minute*2 {
		return fmt.Sprintf("%.0f seconds", d.Seconds())
	}
	if d < time.Minute*10 {
		return fmt.Sprintf("%.1f minutes", d.Seconds()/60)
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0f minutes", d.Seconds()/60)
	}
	if d < time.Hour*4 {
		return fmt.Sprintf("%.1f hours", d.Seconds()/3600)
	}
	if d < time.Hour*60 {
		return fmt.Sprintf("%.0f hours", d.Seconds()/3600)
	}
	if d < time.Hour*24*1000 {
		return fmt.Sprintf("%.0f days", d.Seconds()/3600/24)
	}
	return fmt.Sprintf("%.1f years", d.Seconds()/3600/24/365.25)
}

func (e *etc) Estimate(stats *Stats) (time.Duration, error) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	// assume concurrency is the same as the number in-progress.
	// This is only incorrect when the queue is empty, when it doesn't matter
	if len(e.failures)+len(e.successes) == 0 {
		return time.Duration(0), errors.New("no sample data")
	}
	if stats.InProgress.Load()+stats.Queued.Load() == 0 {
		return time.Duration(0), nil
	}
	pSuccess := float64(len(e.successes)) / float64(len(e.successes)+len(e.failures))
	var meanSuccess time.Duration
	var meanFailure time.Duration
	var maxSuccess time.Duration
	var maxFailure time.Duration
	for _, success := range e.successes {
		if success > maxSuccess {
			maxSuccess = success
		}
		meanSuccess = meanSuccess + success
	}
	for _, failure := range e.failures {
		if failure > maxFailure {
			maxFailure = failure
		}
		meanFailure = meanFailure + failure
	}
	// convert from total duration to average duration
	if len(e.successes) > 0 {
		meanSuccess = meanSuccess / time.Duration(len(e.successes))
	}
	if len(e.failures) > 0 {
		meanFailure = meanFailure / time.Duration(len(e.failures))
	}

	// weighted mean job duration:
	wDurationSeconds := meanSuccess.Seconds()*pSuccess + meanFailure.Seconds()*(1-pSuccess)
	logger.Debug("estimated mean", slog.Float64("weighted duration (seconds)", wDurationSeconds), slog.Float64("success", pSuccess), slog.Duration("mean success", meanSuccess), slog.Duration("mean failure", meanFailure))
	// fudge the weighted duration if we have fewer samples than WIP. as we are biased towards the jobs which take less time
	if wip := stats.InProgress.Load(); int(wip) > len(e.successes)+len(e.failures) {
		wDurationSeconds *= float64(wip) / float64(len(e.successes)+len(e.failures))
	}

	// weighted max time
	wMaxDuration := time.Duration((maxSuccess.Seconds()*pSuccess + maxFailure.Seconds()*(1-pSuccess)) * float64(time.Second))
	logger.Debug("estimated max", slog.Duration("weighted maximum duration", wMaxDuration), slog.Float64("success", pSuccess), slog.Duration("maximum success", maxSuccess), slog.Duration("maximum failure", maxFailure))
	emptiedAt, isEmpty := stats.queueEmptyTime()
	if !isEmpty {
		// estimate queue empty time: number of queued items * weighted job run time
		qet := time.Duration(wDurationSeconds * float64(stats.Queued.Load()) / float64(e.concurrency) * float64(time.Second))
		if lowerLimit := time.Duration(stats.Queued.Load()) * e.minimumDuration; lowerLimit > qet {
			qet = lowerLimit
		}
		return qet + wMaxDuration, nil
	}
	// The queue has drained, so only the jobs still running remain.
	return wMaxDuration - time.Since(emptiedAt), nil
}

func NewEtc(concurrency int, minimumDuration time.Duration) *etc {
	return &etc{successes: make([]time.Duration, 0, 100), failures: make([]time.Duration, 0, 100), mutex: new(sync.RWMutex), concurrency: concurrency, minimumDuration: minimumDuration}
}

func (e *etc) AddSuccess(d time.Duration) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.successes = append(e.successes, d)
}

func (e *etc) AddFailure(d time.Duration) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.failures = append(e.failures, d)
}
