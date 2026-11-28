package bench2

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// cpuSeconds returns user+sys CPU of the whole process (all threads), so
// that any in-process worker threads are accounted. Platform files implement it.
func cpuSeconds() float64 { return CPUSeconds() }

// resetPeakRSS resets the kernel's high-water mark (Linux: write "5" to
// /proc/self/clear_refs) so VmHWM reflects only the next workload.
func resetPeakRSS() {
	_ = os.WriteFile("/proc/self/clear_refs", []byte("5"), 0)
}

func statusKB(key string) int64 {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), key+":") {
			fs := strings.Fields(sc.Text())
			if len(fs) >= 2 {
				n, _ := strconv.ParseInt(fs[1], 10, 64)
				return n
			}
		}
	}
	return 0
}

// peakRSSMiB returns VmHWM (peak since last reset) in MiB.
func peakRSSMiB() float64 { return float64(statusKB("VmHWM")) / 1024 }

// rssSampler polls VmRSS as a fallback where clear_refs is unavailable.
type rssSampler struct {
	stop chan struct{}
	max  int64
}

func startRSSSampler() *rssSampler {
	s := &rssSampler{stop: make(chan struct{})}
	go func() {
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				if v := statusKB("VmRSS"); v > s.max {
					s.max = v
				}
			}
		}
	}()
	return s
}

func (s *rssSampler) stopMiB() float64 {
	close(s.stop)
	return float64(s.max) / 1024
}
