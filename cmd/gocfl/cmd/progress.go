package cmd

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const rate = time.Duration(250 * time.Millisecond)
const stalledDur = time.Second * 10

// max width for size: "999.99 GB "
var sizeStyle = lipgloss.NewStyle().
	Width(10).
	Bold(true)
	// Foreground(lipgloss.Color("202"))

// max width for rate: "[999.99 MB/s]"
var rateStyle = lipgloss.NewStyle().
	Width(13).
	Foreground(lipgloss.Color("#999999"))

var statusStyle = lipgloss.NewStyle().
	Width(12).
	Foreground(lipgloss.Color("#999999"))

var sizeUnits = []string{"B", "KB", "MB", "GB", "TB"}
var durationUnits = []string{"s", "m", "h"}

type ProgressWriter struct {
	preamble  string
	lock      sync.RWMutex
	total     int64
	startTime time.Time
	lastWrite time.Time
	stats     scaledStats
}

type scaledStats struct {
	Total     float64
	TotalUnit string
	Rate      float64
	RateUnit  string
}

func (pw *ProgressWriter) Total() int64 {
	return pw.total
}

func (pw *ProgressWriter) Start(fn func(w io.Writer) error) error {
	errch := make(chan error)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			pw.updateStats()
			select {
			case err := <-errch:
				pw.draw(true, err)
				return
			default:
				pw.draw(false, nil)
				time.Sleep(rate)
			}
		}
	}()
	err := fn(pw)
	errch <- err
	wg.Wait()
	return err
}

func (pw *ProgressWriter) updateStats() {
	elapsed := pw.lastWrite.Sub(pw.startTime)
	pw.stats.Total, pw.stats.TotalUnit = scaleSize(float64(pw.total))
	pw.stats.Rate, pw.stats.RateUnit = scaleSize(float64(pw.total) / elapsed.Seconds())
}

func (pw *ProgressWriter) Write(buff []byte) (int, error) {
	l := len(buff)
	pw.lock.Lock()
	defer pw.lock.Unlock()
	pw.lastWrite = time.Now()
	if pw.total == 0 && l > 0 {
		pw.startTime = pw.lastWrite
	}
	pw.total += int64(l)
	return l, nil
}

func (pw *ProgressWriter) draw(final bool, err error) {
	pw.lock.RLock()
	defer pw.lock.RUnlock()
	var status string
	switch [2]bool{final, err == nil} {
	case [2]bool{false, true}:
		if !pw.lastWrite.IsZero() && time.Since(pw.lastWrite) > stalledDur {
			status = " - stalled"
			break
		}
		status = " - running"
	case [2]bool{true, true}:
		status = " - done!"
	case [2]bool{true, false}:
		status = " - stopped"
	}
	sizeMsg := sizeStyle.Render(fmt.Sprintf("%.2f %s", pw.stats.Total, pw.stats.TotalUnit))
	rateMsg := rateStyle.Render(fmt.Sprintf("[%.2f %s/s]", pw.stats.Rate, pw.stats.RateUnit))
	statusMsg := statusStyle.Render(status)
	term := "\r"
	if final {
		term = "\n"
	}
	fmt.Print(pw.preamble + sizeMsg + rateMsg + statusMsg + term)
}
func scaleSize(n float64) (float64, string) {
	unit := sizeUnits[0]
	for _, u := range sizeUnits[1:] {
		if n < 1000 {
			break
		}
		n = n / 1000
		unit = u
	}
	return n, unit
}

func scaleDuration(d time.Duration) (float64, string) {
	unit := durationUnits[0]
	val := d.Seconds()
	for _, u := range durationUnits[1:] {
		if val < 60 {
			break
		}
		val = val / 60
		unit = u
	}
	return val, unit
}
