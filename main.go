// MIT License

// Copyright (c) 2021 Thomas Weber, pascom GmbH & Co. Kg

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"gopkg.in/urfave/cli.v1"
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
		}, cli.StringFlag{
			Name:   "rtpmetricsPath",
			Value:  "",
			Usage:  "The http scrape path for rtpengine metrics",
			EnvVar: "RTPMETRICS_PATH",
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
	// and register it in prometheus API
	prometheus.MustRegister(collector)

	metricsPath := c.String("metricsPath")
	listenAddress := fmt.Sprintf("%s:%d", c.String("bindIp"), c.Int("bindPort"))
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
	rtpmetricsPath := c.String("rtpmetricsPath")
	if rtpmetricsPath != "" {
		log.Info("Enabling rtp metrics @", rtpmetricsPath)
		http.HandleFunc(rtpmetricsPath, func(w http.ResponseWriter, r *http.Request) {
			resp, err := http.Get("http://127.0.0.1:9901/metrics")
			if err != nil {
				log.Error(err)
				http.Error(w,
					fmt.Sprintf("Failed to connect to rtpengine: %s", err.Error()),
					http.StatusServiceUnavailable)
				return
			}
			defer resp.Body.Close()
			resp2, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Error(err)
				http.Error(w,
					fmt.Sprintf("Failed to read response from rtpengine: %s", err.Error()),
					http.StatusInternalServerError)
				return
			}
			w.Write(resp2)
		})
	}

	// wire "/metrics" -> prometheus API collectors
	http.HandleFunc(metricsPath, promhttp.Handler().ServeHTTP)

	// start http server
	log.Info("Listening on ", listenAddress, metricsPath)
	return http.ListenAndServe(listenAddress, nil)
}
