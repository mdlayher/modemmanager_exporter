package modemmanagerexporter

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/mdlayher/metricslite"
	"github.com/mdlayher/modemmanager"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// Prometheus metric names.
	mmInfo                  = "modemmanager_info"
	mmModemInfo             = "modemmanager_modem_info"
	mmModemNetworkPortInfo  = "modemmanager_modem_network_port_info"
	mmModemNetworkTimestamp = "modemmanager_network_timestamp_seconds"
	mmModemPowerState       = "modemmanager_modem_power_state"
	mmModemState            = "modemmanager_modem_state"
	mmModemSignalLTERSRQ    = "modemmanager_modem_signal_lte_rsrq_db"
	mmModemSignalLTERSRP    = "modemmanager_modem_signal_lte_rsrp_dbm"
	mmModemSignalLTERSSI    = "modemmanager_modem_signal_lte_rssi_dbm"
	mmModemSignalLTESNR     = "modemmanager_modem_signal_lte_snr_db"
)

// NewHandler returns an http.Handler that serves Prometheus metrics gathered
// using a ModemManager client.
func NewHandler(reg *prometheus.Registry, c *modemmanager.Client) http.Handler {
	mm := metricslite.NewPrometheus(reg)

	// Each scrape will use the MM client to fetch data.
	register(mm)
	mm.OnConstScrape(onScrape(c))

	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}

// register registers the exporter's metrics with the input metrics interface.
func register(mm metricslite.Interface) {
	mm.ConstGauge(
		mmInfo,
		"Metadata about the ModemManager daemon.",
		"version",
	)

	mm.ConstGauge(
		mmModemInfo,
		"Metadata about a managed modem.",
		"device_id", "firmware", "imei", "model",
	)

	mm.ConstGauge(
		mmModemNetworkPortInfo,
		"Metadata about the attached network interface ports for a modem. Note that device refers to the network interface name, and not the modem name.",
		"device_id", "device",
	)

	mm.ConstGauge(
		mmModemNetworkTimestamp,
		"The current UNIX timestamp as reported by a modem's cellular network.",
		"device_id",
	)

	mm.ConstGauge(
		mmModemPowerState,
		"An enumeration of power states for a modem, where a value of 1 indicates the current state.",
		"device_id", "state",
	)

	mm.ConstGauge(
		mmModemState,
		"An enumeration of cellular connection states for a modem, where a value of 1 indicates the current state.",
		"device_id", "state",
	)

	mm.ConstGauge(
		mmModemSignalLTERSRQ,
		"A modem's current LTE signal RSRQ (Reference Signal Received Quality) in dB.",
		"device_id",
	)

	mm.ConstGauge(
		mmModemSignalLTERSRP,
		"A modem's current LTE signal RSRP (Reference Signal Received Power) in dBm.",
		"device_id",
	)

	mm.ConstGauge(
		mmModemSignalLTERSSI,
		"A modem's current LTE signal RSSI (Received Signal Strength Indication) in dBm.",
		"device_id",
	)

	mm.ConstGauge(
		mmModemSignalLTESNR,
		"A modem's current LTE signal SNR (Signal-to-Noise Ratio) in dB.",
		"device_id",
	)
}

// onScrape returns a metricslite.ScrapeFunc which uses a MM client to gather
// metrics.
func onScrape(c *modemmanager.Client) metricslite.ScrapeFunc {
	return func(metrics map[string]func(value float64, labels ...string)) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := c.ForEachModem(ctx, func(ctx context.Context, m *modemmanager.Modem) error {
			// Perform any necessary calls before exporting any metrics.
			var nowp *time.Time
			now, err := m.GetNetworkTime(ctx)
			if err != nil {
				nowp = nil
			} else {
				nowp = &now
			}

			s, err := m.Signal(ctx)
			if err != nil {
				return fmt.Errorf("failed to get signal strength: %v", err)
			}

			scrape(metrics, m, nowp, s)
			return nil
		})
		if err != nil {
			return &metricslite.ScrapeError{
				Metric: mmInfo,
				Err:    err,
			}
		}

		// Export MM metadata outside the loop so it'll be present even if no
		// modems are detected.
		metrics[mmInfo](1.0, c.Version)

		return nil
	}
}

// scrape performs a single metrics collection pass for one modem and its data.
func scrape(metrics map[string]func(value float64, labels ...string), m *modemmanager.Modem, now *time.Time, s *modemmanager.Signal) {
	// Device ID is used as the unique key on metrics.
	id := m.DeviceIdentifier

	for name, c := range metrics {
		switch name {
		case mmInfo:
			// Skip, handled outside this loop.
		case mmModemInfo:
			c(1.0, id, m.Revision, m.EquipmentIdentifier, m.Model)
		case mmModemNetworkPortInfo:
			portInfo(c, m)
		case mmModemNetworkTimestamp:
			if now != nil {
				c(float64(now.Unix()), id)
			}
		case mmModemPowerState:
			powerState(c, m)
		case mmModemState:
			state(c, m)
		case mmModemSignalLTERSRP:
			c(s.LTE.RSRP, id)
		case mmModemSignalLTERSRQ:
			c(s.LTE.RSRQ, id)
		case mmModemSignalLTERSSI:
			c(s.LTE.RSSI, id)
		case mmModemSignalLTESNR:
			c(s.LTE.SNR, id)
		default:
			panicf("modemmanager_exporter: unhandled metric %q", name)
		}
	}
}

// portInfo collects a Modem's network port info metrics.
func portInfo(c func(value float64, labels ...string), m *modemmanager.Modem) {
	for _, p := range m.Ports {
		// Only export information about network ports because they can be
		// joined with other metrics such as those from node_exporter. It isn't
		// clear that exporting AT, MBIM, etc. would be useful at this point.
		if p.Type != modemmanager.PortTypeNet {
			continue
		}

		c(1.0, m.DeviceIdentifier, p.Name)
	}
}

// powerState collects a Modem's power state metrics as an enum.
func powerState(c func(value float64, labels ...string), m *modemmanager.Modem) {
	states := []struct {
		s  string
		ps modemmanager.PowerState
	}{
		{
			s:  "unknown",
			ps: modemmanager.PowerStateUnknown,
		},
		{
			s:  "off",
			ps: modemmanager.PowerStateOff,
		},
		{
			s:  "low",
			ps: modemmanager.PowerStateLow,
		},
		{
			s:  "on",
			ps: modemmanager.PowerStateOn,
		},
	}

	// Export all power states but note the active one with a value of 1.0.
	for _, s := range states {
		var f float64
		if s.ps == m.PowerState {
			f = 1.0
		}

		c(f, m.DeviceIdentifier, s.s)
	}
}

// state collects a Modem's state metrics as an enum.
func state(c func(value float64, labels ...string), m *modemmanager.Modem) {
	states := []struct {
		s  string
		st modemmanager.State
	}{
		{
			s:  "failed",
			st: modemmanager.StateFailed,
		},
		{
			s:  "unknown",
			st: modemmanager.StateUnknown,
		},
		{
			s:  "locked",
			st: modemmanager.StateLocked,
		},
		{
			s:  "disabled",
			st: modemmanager.StateDisabled,
		},
		{
			s:  "disabling",
			st: modemmanager.StateDisabling,
		},
		{
			s:  "enabling",
			st: modemmanager.StateEnabling,
		},
		{
			s:  "enabled",
			st: modemmanager.StateEnabled,
		},
		{
			s:  "searching",
			st: modemmanager.StateSearching,
		},
		{
			s:  "registered",
			st: modemmanager.StateRegistered,
		},
		{
			s:  "disconnecting",
			st: modemmanager.StateDisconnecting,
		},
		{
			s:  "connecting",
			st: modemmanager.StateConnecting,
		},
		{
			s:  "connected",
			st: modemmanager.StateConnected,
		},
	}

	// Export all power states but note the active one with a value of 1.0.
	for _, s := range states {
		var f float64
		if s.st == m.State {
			f = 1.0
		}

		c(f, m.DeviceIdentifier, s.s)
	}
}

func panicf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}
