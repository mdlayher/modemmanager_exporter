// Command modemmanager_exporter implements a Prometheus exporter for
// ModemManager and its devices.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mdlayher/modemmanager"
	modemmanagerexporter "github.com/mdlayher/modemmanager_exporter"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	var (
		addr = flag.String("addr", ":9539", "address for ModemManager exporter")
		rate = flag.Duration("rate", 5*time.Second, "how frequently ModemManager should poll each modem for its extended signal strength data")
	)

	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Keep the D-Bus connection open for the lifetime of the program, and use
	// it immediately to start polling the modems for signal status.
	c, err := modemmanager.Dial(ctx)
	if err != nil {
		log.Fatalf("failed to connect to ModemManager: %v", err)
	}
	defer c.Close()

	err = c.ForEachModem(ctx, func(ctx context.Context, m *modemmanager.Modem) error {
		log.Printf("modem %d: %q", m.Index, m.Model)
		if err := m.SignalSetup(ctx, *rate); err != nil {
			return fmt.Errorf("failed to set signal refresh rate: %v", err)
		}

		return nil
	})
	if err != nil {
		log.Fatalf("failed to configure modems: %v", err)
	}

	// Set up the Prometheus registry and exporter handler.
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(
		prometheus.NewBuildInfoCollector(),
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)

	mux := http.NewServeMux()
	mux.Handle("/metrics", modemmanagerexporter.NewHandler(reg, c))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/metrics", http.StatusMovedPermanently)
	})

	log.Printf("starting ModemManager exporter on %q", *addr)

	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("cannot start ModemManager exporter: %v", err)
	}
}
