package cmd

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const rate = time.Duration(250 * time.Millisecond)
const stalledDur = time.Second * 30

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

//var durationUnits = []string{"s", "m", "h"}

type ProgressWriter struct {
	total     atomic.Int64
	preamble  string
	lastTotal int64
	startTime time.Time
	lastWrite time.Time // for stall detection
}

func (pw *ProgressWriter) Write(buff []byte) (int, error) {
	l := len(buff)
	pw.total.Add(int64(l))
	return l, nil
}

func (pw *ProgressWriter) Total() int64 {
	return pw.total.Load()
}

func (pw *ProgressWriter) Start(fn func(w io.Writer) error) error {
	errch := make(chan error)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
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

type scaledStats struct {
	Total     float64
	TotalUnit string
	Rate      float64
	RateUnit  string
}

func (pw *ProgressWriter) scaledStats() scaledStats {
	var stats scaledStats
	elapsed := time.Since(pw.startTime)
	stats.Total, stats.TotalUnit = scaleSize(float64(pw.total.Load()))
	stats.Rate, stats.RateUnit = scaleSize(float64(pw.total.Load()) / elapsed.Seconds())
	return stats
}

func (pw *ProgressWriter) draw(final bool, err error) {
	newTotal := pw.total.Load()
	if pw.lastTotal == 0 && newTotal > 0 {
		// writes have started
		pw.startTime = time.Now()
	}
	if pw.lastTotal != newTotal {
		pw.lastWrite = time.Now()
	}
	status := "running..."
	if err != nil {
		status = "stopping"
	} else if !final && time.Since(pw.lastWrite) > stalledDur {
		status = "stalled"
	} else if final {
		status = "done"
	}
	stats := pw.scaledStats()
	sizeMsg := sizeStyle.Render(fmt.Sprintf("%.2f %s", stats.Total, stats.TotalUnit))
	rateMsg := rateStyle.Render(fmt.Sprintf("[%.2f %s/s]", stats.Rate, stats.RateUnit))
	statusMsg := statusStyle.Render(status)
	term := "\r"
	if final {
		term = "\n"
	}
	fmt.Print(pw.preamble + sizeMsg + rateMsg + statusMsg + term)
	pw.lastTotal = newTotal
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

// func scaleDuration(d time.Duration) (float64, string) {
// 	unit := durationUnits[0]
// 	val := d.Seconds()
// 	for _, u := range durationUnits[1:] {
// 		if val < 60 {
// 			break
// 		}
// 		val = val / 60
// 		unit = u
// 	}
// 	return val, unit
// }
