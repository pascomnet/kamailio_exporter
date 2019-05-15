package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"gopkg.in/urfave/cli.v1"
	"net/http"
	"os"
)

var Version string

func main() {
	app := cli.NewApp()
	app.Name = "Kamailio exporter"
	app.Usage = "Expose Kamailio statistics as http endpoint for prometheus."
	app.Version = Version
	// define cli flags
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:   "debug",
			Usage:  "Enable debug logging",
			EnvVar: "DEBUG",
		},
		cli.StringFlag{
			Name:   "socketPath",
			Value:  "/var/run/kamailio/kamailio_ctl",
			Usage:  "Path to Kamailio unix domain socket",
			EnvVar: "SOCKET_PATH",
		},
		cli.StringFlag{
			Name:   "host",
			Usage:  "Kamailio ip or hostname. Domain socket is used if no host is defined.",
			EnvVar: "HOST",
		},
		cli.IntFlag{
			Name:   "port",
			Value:  3012,
			Usage:  "Kamailio port",
			EnvVar: "PORT",
		},
		cli.StringFlag{
			Name:   "bindIp",
			Value:  "0.0.0.0",
			Usage:  "Listen on this ip for scrape requests",
			EnvVar: "BIND_IP",
		},
		cli.IntFlag{
			Name:   "bindPort",
			Value:  9494,
			Usage:  "Listen on this port for scrape requests",
			EnvVar: "BIND_PORT",
		},
		cli.StringFlag{
			Name:   "metricsPath",
			Value:  "/metrics",
			Usage:  "The http scrape path",
			EnvVar: "METRICS_PATH",
		},
	}
	app.Action = appAction
	// then start the application
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

// start the application
func appAction(c *cli.Context) error {
	log.Info("Starting kamailio exporter")

	if c.Bool("debug") {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging is enabled")
	}

	// create a collector
	collector, err := NewStatsCollector(c)
	if err != nil {
		return err
	}

	// clear default collectors
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry

	// and register it in prometheus API
	prometheus.MustRegister(collector)

	// wire "/" to return some helpful info
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Kamailio Exporter</title></head>
             <body>
			 <p>This is a prometheus metric exporter for Kamailio.</p>
			 <p>Browse <a href='` + metricsPath + `'>` + metricsPath + `</a> 
			 to get the metrics.</p>
             </body>
             </html>`))
	})

	// wire "/metrics" -> prometheus API collectors
	httpHandler := promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{})
	http.HandleFunc(c.String("metricsPath"), httpHandler.ServeHTTP)

	// start http server
	listenAddress := fmt.Sprintf("%s:%d", c.String("bindIp"), c.Int("bindPort"))
	log.Info("Listening on ", listenAddress, metricsPath)
	return http.ListenAndServe(listenAddress, nil)
}
