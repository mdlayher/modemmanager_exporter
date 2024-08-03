package modemmanagerexporter

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/mdlayher/metricslite"
	"github.com/mdlayher/modemmanager"
)

func TestMetrics(t *testing.T) {
	mm := metricslite.NewMemory()
	register(mm)

	// Scrape metrics into memory using canned data so we can compare against
	// known outputs.
	mm.OnConstScrape(func(metrics map[string]func(value float64, labels ...string)) error {
		var s modemmanager.Signal
		s.LTE.RSRP = -116
		s.LTE.RSRQ = -17
		s.LTE.RSSI = -81
		s.LTE.SNR = 1
		now := time.Unix(1, 0)

		scrape(
			metrics,
			&modemmanager.Modem{
				DeviceIdentifier:    "foo",
				EquipmentIdentifier: "deadbeef",
				Model:               "Test Modem",
				Ports: []modemmanager.Port{
					{
						Name: "ttyUSB0",
						Type: modemmanager.PortTypeAT,
					},
					{
						Name: "wwan0",
						Type: modemmanager.PortTypeNet,
					},
				},
				PowerState: modemmanager.PowerStateOn,
				State:      modemmanager.StateConnected,
				Revision:   "2020-07-17",
			},
			&now,
			&s,
		)
		return nil
	})

	want := map[string]metricslite.Series{
		mmInfo: {
			// Never collected because this metric is not per-modem.
			Samples: map[string]float64{},
		},
		mmModemInfo: {
			Samples: map[string]float64{"device_id=foo,firmware=2020-07-17,imei=deadbeef,model=Test Modem": 1},
		},
		mmModemNetworkPortInfo: {
			Samples: map[string]float64{"device_id=foo,device=wwan0": 1},
		},
		mmModemPowerState: {
			Samples: map[string]float64{
				"device_id=foo,state=low":     0,
				"device_id=foo,state=off":     0,
				"device_id=foo,state=on":      1,
				"device_id=foo,state=unknown": 0,
			},
		},
		mmModemState: {
			Samples: map[string]float64{
				"device_id=foo,state=connected":     1,
				"device_id=foo,state=connecting":    0,
				"device_id=foo,state=disabled":      0,
				"device_id=foo,state=disabling":     0,
				"device_id=foo,state=disconnecting": 0,
				"device_id=foo,state=enabled":       0,
				"device_id=foo,state=enabling":      0,
				"device_id=foo,state=failed":        0,
				"device_id=foo,state=locked":        0,
				"device_id=foo,state=registered":    0,
				"device_id=foo,state=searching":     0,
				"device_id=foo,state=unknown":       0,
			},
		},
		mmModemSignalLTERSRP: {
			Samples: map[string]float64{"device_id=foo": -116},
		},
		mmModemSignalLTERSRQ: {
			Samples: map[string]float64{"device_id=foo": -17},
		},
		mmModemSignalLTERSSI: {
			Samples: map[string]float64{"device_id=foo": -81},
		},
		mmModemSignalLTESNR: {
			Samples: map[string]float64{"device_id=foo": 1},
		},
		mmModemNetworkTimestamp: {
			Samples: map[string]float64{"device_id=foo": 1},
		},
	}

	// Clear metrics names and help strings from the output so we can more
	// concisely test the sample data.
	got := mm.Series()
	for k, v := range got {
		v.Name = ""
		v.Help = ""
		got[k] = v
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("unexpected timeseries (-want +got):\n%s", diff)
	}
}
