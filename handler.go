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
	mmModemNetworkTimestamp = "modemmanager_network_timestamp_seconds"
	mmModemSignalLTERSRQ    = "modemmanager_modem_signal_lte_rsrq_db"
	mmModemSignalLTERSRP    = "modemmanager_modem_signal_lte_rsrp_dbm"
	mmModemSignalLTERSSI    = "modemmanager_modem_signal_lte_rssi_dbm"
	mmModemSignalLTESNR     = "modemmanager_modem_signal_lte_snr_db"
)

// NewHandler returns an http.Handler that serves Prometheus metrics gathered
// using a ModemManager client.
func NewHandler(reg *prometheus.Registry, c *modemmanager.Client) http.Handler {
	mm := metricslite.NewPrometheus(reg)

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
		mmModemNetworkTimestamp,
		"The current UNIX timestamp as reported by a modem's cellular network.",
		"device_id",
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

	// Each scrape will use the MM client to fetch data.
	mm.OnConstScrape(onScrape(c))

	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}

// onScrape returns a metricslite.ScrapeFunc which uses a MM client to gather
// metrics.
func onScrape(c *modemmanager.Client) metricslite.ScrapeFunc {
	return func(metrics map[string]func(value float64, labels ...string)) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := c.ForEachModem(ctx, func(ctx context.Context, m *modemmanager.Modem) error {
			// Perform any necessary calls before exporting any metrics.
			now, err := m.GetNetworkTime(ctx)
			if err != nil {
				return fmt.Errorf("failed to get network time: %v", err)
			}

			s, err := m.Signal(ctx)
			if err != nil {
				return fmt.Errorf("failed to get signal strength: %v", err)
			}

			// Device ID is used as the unique key on metrics.
			id := m.DeviceIdentifier

			for name, c := range metrics {
				switch name {
				case mmInfo:
					// Skip, handled outside this loop.
				case mmModemInfo:
					c(1.0, id, m.Revision, m.EquipmentIdentifier, m.Model)
				case mmModemNetworkTimestamp:
					c(float64(now.Unix()), id)
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

func panicf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}
