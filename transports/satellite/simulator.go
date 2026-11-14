package satellite

import (
	"encoding/json"
	"time"

	"github.com/islandweave/islandweave/core/daemon"
	"github.com/islandweave/islandweave/protocols/dtn"
	"github.com/islandweave/islandweave/protocols/iwst"
)

// SimulatedAdapter implements iwst.GroundStationAdapter for testing and simulation.
type SimulatedAdapter struct {
	StationName string
	Window      iwst.ContactWindow
}

func (a *SimulatedAdapter) Name() string             { return a.StationName }
func (a *SimulatedAdapter) LinkCapacityBps() int64   { return a.Window.BandwidthBps }
func (a *SimulatedAdapter) MaxBytesPerPass() int64   { return a.Window.MaxBytes }
func (a *SimulatedAdapter) IsPassActive(t time.Time) bool {
	u := t.Unix()
	return u >= a.Window.StartTime && u <= a.Window.EndTime
}

// RunContactSimulation simulates transferring queued DTN bundles from sender to recipient
// across one or more bandwidth-limited, intermittent satellite contact windows.
// Partial bundle segments interrupted at the end of a pass resume cleanly in subsequent passes.
func RunContactSimulation(sender, recipient *daemon.Daemon, passes []iwst.ContactWindow) ([]iwst.PassLog, error) {
	bundles, err := sender.ExportForwardableBundles()
	if err != nil {
		return nil, err
	}

	// Pre-serialize each bundle into a byte stream segment queue
	type txItem struct {
		raw    []byte
		offset int
	}
	queue := make([]txItem, 0, len(bundles))
	for i := range bundles {
		raw, err := json.Marshal(bundles[i])
		if err != nil {
			return nil, err
		}
		queue = append(queue, txItem{raw: raw})
	}

	var logs []iwst.PassLog
	idx := 0
	for _, pass := range passes {
		logEntry := iwst.PassLog{
			PassID:        pass.PassID,
			SatelliteName: pass.SatelliteName,
			CompletedAt:   time.Now().Unix(),
		}
		budget := pass.MaxBytes
		for idx < len(queue) && budget > 0 {
			item := &queue[idx]
			rem := int64(len(item.raw) - item.offset)
			if rem <= budget {
				// Full remaining segment of this bundle fits in this pass
				budget -= rem
				logEntry.BytesTransferred += rem
				item.offset = len(item.raw)

				var b dtn.Bundle
				if err := json.Unmarshal(item.raw, &b); err == nil {
					if _, err := recipient.IngestExternalBundle(&b); err == nil {
						logEntry.BundlesDelivered++
					}
				}
				idx++
			} else {
				// Pass ends mid-bundle; save partial progress for next contact window
				item.offset += int(budget)
				logEntry.BytesTransferred += budget
				logEntry.Interrupted = true
				budget = 0
			}
		}
		if idx < len(queue) && !logEntry.Interrupted && logEntry.BytesTransferred > 0 {
			logEntry.Interrupted = true
		}
		logs = append(logs, logEntry)
		if idx >= len(queue) {
			break
		}
	}
	return logs, nil
}
