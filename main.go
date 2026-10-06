package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	version = "1.0.0"

	configFile  = flag.String("config.file", "", "Path to configuration file (YAML)")
	listenAddr  = flag.String("listen", ":9108", "Address to listen on")
	webPath     = flag.String("web.path", "/metrics", "Path for metrics endpoint")
	showVersion = flag.Bool("version", false, "Print version and exit")

	sc       = &SafeConfig{C: &Config{Modules: map[string]ModuleConfig{"default": defaultModule}}}
	reloadCh chan chan error
	logger   *slog.Logger
)

func main() {
	flag.Parse()
	if *showVersion {
		fmt.Println("gooxi-exporter", version)
		return
	}
	logger = slog.New(slog.NewTextHandler(os.Stdout, nil))

	if err := sc.ReloadConfig(*configFile); err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// SIGHUP reload
	hup := make(chan os.Signal, 1)
	reloadCh = make(chan chan error)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-hup:
				if err := sc.ReloadConfig(*configFile); err != nil {
					logger.Error("config reload failed", "error", err)
				} else {
					logger.Info("config reloaded")
				}
			case rc := <-reloadCh:
				if err := sc.ReloadConfig(*configFile); err != nil {
					logger.Error("config reload failed", "error", err)
					rc <- err
				} else {
					rc <- nil
				}
			}
		}
	}()

	mux := http.NewServeMux()
	mux.Handle(*webPath, scrapeHandler(categoryAll))
	mux.Handle(*webPath+"/sensors", scrapeHandler(categorySensors))
	mux.Handle(*webPath+"/health", scrapeHandler(categoryHealth))
	mux.HandleFunc("/-/reload", reloadHandler)
	mux.HandleFunc("/", indexHandler)

	logger.Info("gooxi-exporter starting", "version", version, "listen", *listenAddr, "web_path", *webPath)
	logFatal := func(err error) {
		if err != nil {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}
	logFatal(http.ListenAndServe(*listenAddr, mux))
}

// scrapeHandler implements the multi-target exporter pattern.
// The target BMC host is passed via ?target=<host> query parameter.
// The category selects which metric groups the scrape produces.
func scrapeHandler(category scrapeCategory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("target")
		if target == "" {
			http.Error(w, "'target' parameter must be specified", http.StatusBadRequest)
			return
		}

		module := r.URL.Query().Get("module")
		if module == "" {
			module = "default"
		}

		logger.Debug("scrape", "target", target, "module", module, "category", string(category))

		registry := prometheus.NewRegistry()
		collector := newGooxiCollector(target, module, category, sc)
		registry.MustRegister(collector)

		promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(w, r)
	}
}

func reloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	rc := make(chan error)
	reloadCh <- rc
	if err := <-rc; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func indexHandler(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprintf(w, `<html><head><title>Gooxi Exporter</title></head>
<body>
<h1>Gooxi BMC Exporter</h1>
<p>Endpoints:</p>
<ul>
  <li><code>%s?target=&lt;host&gt;</code> — all metrics</li>
  <li><code>%s/sensors?target=&lt;host&gt;</code> — sensor readings only</li>
  <li><code>%s/health?target=&lt;host&gt;</code> — chassis power and uptime</li>
</ul>
<form action="%s">
  <label>Target BMC:</label> <input type="text" name="target" placeholder="192.168.0.1"><br>
  <label>Module:</label> <input type="text" name="module" value="default"><br>
  <input type="submit" value="Scrape">
</form>
</body></html>`, *webPath, *webPath, *webPath, *webPath)
}
