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
	"net"
	"strconv"
	"strings"

	binrpc "github.com/florentchauveau/go-kamailio-binrpc/v3"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"gopkg.in/urfave/cli.v1"
)

// declare a series of prometheus metric descriptions
// we can reuse them for each scrape
var (
	core_request_total = prometheus.NewDesc(
		"kamailio_core_request_total",
		"Request counters",
		[]string{"method"}, nil)

	core_rcv_request_total = prometheus.NewDesc(
		"kamailio_core_rcv_request_total",
		"Received requests by method",
		[]string{"method"}, nil)

	core_reply_total = prometheus.NewDesc(
		"kamailio_core_reply_total",
		"Reply counters",
		[]string{"type"}, nil)

	core_rcv_reply_total = prometheus.NewDesc(
		"kamailio_core_rcv_reply_total",
		"Received replies by code",
		[]string{"code"}, nil)

	shmem_bytes = prometheus.NewDesc(
		"kamailio_shm_bytes",
		"Shared memory sizes",
		[]string{"type"}, nil)

	shmem_fragments = prometheus.NewDesc(
		"kamailio_shm_fragments",
		"Shared memory fragment count",
		[]string{}, nil)

	dns_failed = prometheus.NewDesc(
		"kamailio_dns_failed_request_total",
		"Failed dns requests",
		[]string{}, nil)

	bad_uri = prometheus.NewDesc(
		"kamailio_bad_uri_total",
		"Messages with bad uri",
		[]string{}, nil)

	bad_msg_hdr = prometheus.NewDesc(
		"kamailio_bad_msg_hdr",
		"Messages with bad message header",
		[]string{}, nil)

	sl_reply_total = prometheus.NewDesc(
		"kamailio_sl_reply_total",
		"Stateless replies by code",
		[]string{"code"}, nil)

	sl_type_total = prometheus.NewDesc(
		"kamailio_sl_type_total",
		"Stateless replies by type",
		[]string{"type"}, nil)

	tcp_total = prometheus.NewDesc(
		"kamailio_tcp_total",
		"TCP connection counters",
		[]string{"type"}, nil)

	tcp_connections = prometheus.NewDesc(
		"kamailio_tcp_connections",
		"Opened TCP connections",
		[]string{}, nil)

	tcp_writequeue = prometheus.NewDesc(
		"kamailio_tcp_writequeue",
		"TCP write queue size",
		[]string{}, nil)

	tmx_code_total = prometheus.NewDesc(
		"kamailio_tmx_code_total",
		"Completed Transaction counters by code",
		[]string{"code"}, nil)

	tmx_type_total = prometheus.NewDesc(
		"kamailio_tmx_type_total",
		"Completed Transaction counters by type",
		[]string{"type"}, nil)

	tmx = prometheus.NewDesc(
		"kamailio_tmx",
		"Ongoing Transactions",
		[]string{"type"}, nil)

	tmx_rpl_total = prometheus.NewDesc(
		"kamailio_tmx_rpl_total",
		"Tmx reply counters",
		[]string{"type"}, nil)

	dialog = prometheus.NewDesc(
		"kamailio_dialog",
		"Ongoing Dialogs",
		[]string{"type"}, nil)

	pkgmem_used = prometheus.NewDesc(
		"kamailio_pkgmem_used",
		"Private memory used",
		[]string{"entry"},
		nil)

	pkgmem_free = prometheus.NewDesc(
		"kamailio_pkgmem_free",
		"Private memory free",
		[]string{"entry"},
		nil)

	pkgmem_real = prometheus.NewDesc(
		"kamailio_pkgmem_real",
		"Private memory real used",
		[]string{"entry"},
		nil)

	pkgmem_size = prometheus.NewDesc(
		"kamailio_pkgmem_size",
		"Private memory total size",
		[]string{"entry"},
		nil)

	pkgmem_frags = prometheus.NewDesc(
		"kamailio_pkgmem_frags",
		"Private memory total frags",
		[]string{"entry"},
		nil)

	tcp_readers = prometheus.NewDesc(
		"kamailio_tcp_readers",
		"TCP readers",
		[]string{},
		nil)

	tcp_max_connections = prometheus.NewDesc(
		"kamailio_tcp_max_connections",
		"TCP connection limit",
		[]string{},
		nil)

	tls_max_connections = prometheus.NewDesc(
		"kamailio_tls_max_connections",
		"TLS connection limit",
		[]string{},
		nil)

	tls_connections = prometheus.NewDesc(
		"kamailio_tls_connections",
		"Opened TLS connections",
		[]string{},
		nil)

	rtpengine_enabled = prometheus.NewDesc(
		"kamailio_rtpengine_enabled",
		"rtpengine connection status",
		[]string{"url", "set", "index", "weight"},
		nil)
)

type PkgStatsEntry struct {
	entry       int
	used        int
	free        int
	real_used   int
	total_size  int
	total_frags int
}

// the actual Collector object
type StatsCollector struct {
	cliContext   *cli.Context
	socketPath   string
	kamailioHost string
	kamailioPort int
}

// produce a new StatsCollector object
func NewStatsCollector(cliContext *cli.Context) (*StatsCollector, error) {

	// fill the Collector struct
	collector := &StatsCollector{
		cliContext:   cliContext,
		socketPath:   cliContext.String("socketPath"),
		kamailioHost: cliContext.String("host"),
		kamailioPort: cliContext.Int("port"),
	}

	// fine, return the created object struct
	return collector, nil
}

// part of the prometheus.Collector interface
func (c *StatsCollector) Describe(descriptionChannel chan<- *prometheus.Desc) {
	// DescribeByCollect is a helper to implement the Describe method of a custom
	// Collector. It collects the metrics from the provided Collector and sends
	// their descriptors to the provided channel.
	prometheus.DescribeByCollect(c, descriptionChannel)
}

// part of the prometheus.Collector interface
func (c *StatsCollector) Collect(metricChannel chan<- prometheus.Metric) {

	// TODO measure rpc time
	//timer := prometheus.NewTimer(rpc_request_duration)
	//defer timer.ObserveDuration()

	// establish connection to Kamailio server
	var err error
	var conn net.Conn
	if c.kamailioHost == "" {
		log.Debug("Requesting stats from kamailio via domain socket ", c.socketPath)
		conn, err = net.Dial("unix", c.socketPath)
	} else {
		address := fmt.Sprintf("%s:%d", c.kamailioHost, c.kamailioPort)
		log.Debug("Requesting stats from kamailio via binrpc ", address)
		conn, err = net.Dial("tcp", address)
	}
	if err != nil {
		log.Error("Can not connect to kamailio: ", err)
		return
	}

	defer conn.Close()
	// TODO
	// c.conn.SetDeadline(time.Now().Add(c.Timeout))

	// WritePacket returns the cookie generated
	cookie, err := binrpc.WritePacket(conn, "stats.fetch", "all")
	if err != nil {
		log.Error("Can not request stats: ", err)
		return
	}

	// the cookie is passed again for verification
	// we receive records in response
	records, err := binrpc.ReadPacket(conn, cookie)
	if err != nil {
		log.Error("Can not fetch stats: ", err)
		return
	}

	// convert the structure into a simple key=>value map
	items, _ := records[0].StructItems()
	completeStatMap := make(map[string]string)
	for _, item := range items {
		value, _ := item.Value.String()
		completeStatMap[item.Key] = value
	}
	// and produce various prometheus.Metric for well-known stats
	produceMetrics(completeStatMap, metricChannel)
	// produce prometheus.Metric objects for scripted stats (if any)
	convertScriptedMetrics(completeStatMap, metricChannel)

	// now fetch pkg stats
	cookie, err = binrpc.WritePacket(conn, "pkg.stats")
	if err != nil {
		log.Error("Can not request pkg.stats: ", err)
		return
	}

	records, err = binrpc.ReadPacket(conn, cookie)
	if err != nil {
		log.Error("Can not fetch pkg.stats: ", err)
		return
	}

	// convert each pkg entry to a series of metrics
	for _, record := range records {
		items, _ = record.StructItems()
		entry := PkgStatsEntry{}
		for _, item := range items {
			switch item.Key {
			case "entry":
				entry.entry, _ = item.Value.Int()
			case "used":
				entry.used, _ = item.Value.Int()
			case "free":
				entry.free, _ = item.Value.Int()
			case "real_used":
				entry.real_used, _ = item.Value.Int()
			case "total_size":
				entry.total_size, _ = item.Value.Int()
			case "total_frags":
				entry.total_frags, _ = item.Value.Int()
			}
		}
		sentry := strconv.Itoa(entry.entry)
		metricChannel <- prometheus.MustNewConstMetric(pkgmem_used, prometheus.GaugeValue, float64(entry.used), sentry)
		metricChannel <- prometheus.MustNewConstMetric(pkgmem_free, prometheus.GaugeValue, float64(entry.free), sentry)
		metricChannel <- prometheus.MustNewConstMetric(pkgmem_real, prometheus.GaugeValue, float64(entry.real_used), sentry)
		metricChannel <- prometheus.MustNewConstMetric(pkgmem_size, prometheus.GaugeValue, float64(entry.total_size), sentry)
		metricChannel <- prometheus.MustNewConstMetric(pkgmem_frags, prometheus.GaugeValue, float64(entry.total_frags), sentry)
	}

	// fetch tcp details
	cookie, err = binrpc.WritePacket(conn, "core.tcp_info")
	if err != nil {
		log.Error("Can not request core.tcp_info: ", err)
		return
	}

	records, err = binrpc.ReadPacket(conn, cookie)
	if err != nil || len(records) == 0 {
		log.Error("Can not fetch core.tcp_info: ", err)
		return
	}
	items, _ = records[0].StructItems()
	var v int
	for _, item := range items {
		switch item.Key {
		case "readers":
			v, _ = item.Value.Int()
			metricChannel <- prometheus.MustNewConstMetric(tcp_readers, prometheus.GaugeValue, float64(v))
		case "max_connections":
			v, _ = item.Value.Int()
			metricChannel <- prometheus.MustNewConstMetric(tcp_max_connections, prometheus.GaugeValue, float64(v))
		case "max_tls_connections":
			v, _ = item.Value.Int()
			metricChannel <- prometheus.MustNewConstMetric(tls_max_connections, prometheus.GaugeValue, float64(v))
		case "opened_tls_connections":
			v, _ = item.Value.Int()
			metricChannel <- prometheus.MustNewConstMetric(tls_connections, prometheus.GaugeValue, float64(v))
		}
	}

	// fetch rtpengine disabled status and url
	cookie, err = binrpc.WritePacket(conn, "rtpengine.show", "all")
	if err != nil {
		log.Error("Can not request rtpengine.show: ", err)
		return
	}

	records, err = binrpc.ReadPacket(conn, cookie)
	if err != nil || len(records) == 0 {
		log.Error("Can not fetch rtpengine.show: ", err)
		return
	}

	for _, record := range records {
		items, _ = record.StructItems()
		if len(items) == 0 {
			log.Debug("Rtpengine.show all has empty items in record - probably because rtpengine is disabled")
			continue
		}
		var url string
		var setInt, indexInt, weightInt int
		var set, index, weight string
		for _, item := range items {
			switch item.Key {
			case "disabled":
				v, _ = item.Value.Int()
			case "url":
				url, _ = item.Value.String()
			case "set":
				setInt, _ = item.Value.Int()
				set = strconv.Itoa(setInt)
			case "index":
				indexInt, _ = item.Value.Int()
				index = strconv.Itoa(indexInt)
			case "weight":
				weightInt, _ = item.Value.Int()
				weight = strconv.Itoa(weightInt)
			}
		}
		if url == "" {
			log.Error("No valid url found for rtpengine, failed to construct metric rtpengine_enabled")
			continue
		}
		//invert the disabled status to fit the metric name "rtpengine_enabled"
		if v == 1 {
			v = 0
		} else {
			v = 1
		}
		metricChannel <- prometheus.MustNewConstMetric(rtpengine_enabled, prometheus.GaugeValue, float64(v), url, set, index, weight)
	}
}

// produce a series of prometheus.Metric values by converting "well-known" prometheus stats
func produceMetrics(completeStatMap map[string]string, metricChannel chan<- prometheus.Metric) {

	// kamailio_core_request_total
	convertStatToMetric(completeStatMap, "core.drop_requests", "drop", core_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.err_requests", "err", core_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.fwd_requests", "fwd", core_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests", "rcv", core_request_total, metricChannel, prometheus.CounterValue)

	// kamailio_core_rcv_request_total
	convertStatToMetric(completeStatMap, "core.rcv_requests_ack", "ack", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_bye", "bye", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_cancel", "cancel", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_info", "info", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_invite", "invite", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_message", "message", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_notify", "notify", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_options", "options", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_prack", "prack", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_publish", "publish", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_refer", "refer", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_register", "register", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_subscribe", "subscribe", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_requests_update", "update", core_rcv_request_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.unsupported_methods", "unsupported", core_rcv_request_total, metricChannel, prometheus.CounterValue)

	// kamailio_core_reply_total
	convertStatToMetric(completeStatMap, "core.drop_replies", "drop", core_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.err_replies", "err", core_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.fwd_replies", "fwd", core_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies", "rcv", core_reply_total, metricChannel, prometheus.CounterValue)

	// kamailio_core_rcv_reply_total
	convertStatToMetric(completeStatMap, "core.rcv_replies_18x", "18x", core_rcv_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies_1xx", "1xx", core_rcv_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies_2xx", "2xx", core_rcv_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies_3xx", "3xx", core_rcv_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies_401", "401", core_rcv_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies_404", "404", core_rcv_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies_407", "407", core_rcv_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies_408", "408", core_rcv_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies_480", "480", core_rcv_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies_486", "486", core_rcv_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies_4xx", "4xx", core_rcv_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies_5xx", "5xx", core_rcv_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.rcv_replies_6xx", "6xx", core_rcv_reply_total, metricChannel, prometheus.CounterValue)

	// kamailio_shm_bytes
	convertStatToMetric(completeStatMap, "shmem.free_size", "free", shmem_bytes, metricChannel, prometheus.GaugeValue)
	convertStatToMetric(completeStatMap, "shmem.max_used_size", "max_used", shmem_bytes, metricChannel, prometheus.GaugeValue)
	convertStatToMetric(completeStatMap, "shmem.real_used_size", "real_used", shmem_bytes, metricChannel, prometheus.GaugeValue)
	convertStatToMetric(completeStatMap, "shmem.total_size", "total", shmem_bytes, metricChannel, prometheus.GaugeValue)
	convertStatToMetric(completeStatMap, "shmem.used_size", "used", shmem_bytes, metricChannel, prometheus.GaugeValue)

	convertStatToMetric(completeStatMap, "shmem.fragments", "", shmem_fragments, metricChannel, prometheus.GaugeValue)
	convertStatToMetric(completeStatMap, "dns.failed_dns_request", "", dns_failed, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.bad_URIs_rcvd", "", bad_uri, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "core.bad_msg_hdr", "", bad_msg_hdr, metricChannel, prometheus.CounterValue)

	// kamailio_sl_reply_total
	convertStatToMetric(completeStatMap, "sl.1xx_replies", "1xx", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.200_replies", "200", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.202_replies", "202", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.2xx_replies", "2xx", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.300_replies", "300", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.301_replies", "301", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.302_replies", "302", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.3xx_replies", "3xx", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.400_replies", "400", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.401_replies", "401", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.403_replies", "403", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.404_replies", "404", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.407_replies", "407", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.408_replies", "408", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.483_replies", "483", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.4xx_replies", "4xx", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.500_replies", "500", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.5xx_replies", "5xx", sl_reply_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.6xx_replies", "6xx", sl_reply_total, metricChannel, prometheus.CounterValue)

	// kamailio_sl_type_total
	convertStatToMetric(completeStatMap, "sl.failures", "failure", sl_type_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.received_ACKs", "received_ack", sl_type_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.sent_err_replies", "sent_err_reply", sl_type_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.sent_replies", "sent_reply", sl_type_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "sl.xxx_replies", "xxx_reply", sl_type_total, metricChannel, prometheus.CounterValue)

	// kamailio_tcp_total
	convertStatToMetric(completeStatMap, "tcp.con_reset", "con_reset", tcp_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tcp.con_timeout", "con_timeout", tcp_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tcp.connect_failed", "connect_failed", tcp_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tcp.connect_success", "connect_success", tcp_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tcp.established", "established", tcp_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tcp.local_reject", "local_reject", tcp_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tcp.passive_open", "passive_open", tcp_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tcp.send_timeout", "send_timeout", tcp_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tcp.sendq_full", "sendq_full", tcp_total, metricChannel, prometheus.CounterValue)
	// kamailio_tcp_connections
	convertStatToMetric(completeStatMap, "tcp.current_opened_connections", "", tcp_connections, metricChannel, prometheus.GaugeValue)
	// kamailio_tcp_writequeue
	convertStatToMetric(completeStatMap, "tcp.current_write_queue_size", "", tcp_writequeue, metricChannel, prometheus.GaugeValue)

	// kamailio_tmx_code_total
	convertStatToMetric(completeStatMap, "tmx.2xx_transactions", "2xx", tmx_code_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tmx.3xx_transactions", "3xx", tmx_code_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tmx.4xx_transactions", "4xx", tmx_code_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tmx.5xx_transactions", "5xx", tmx_code_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tmx.6xx_transactions", "6xx", tmx_code_total, metricChannel, prometheus.CounterValue)
	// kamailio_tmx_type_total
	convertStatToMetric(completeStatMap, "tmx.UAC_transactions", "uac", tmx_type_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tmx.UAS_transactions", "uas", tmx_type_total, metricChannel, prometheus.CounterValue)
	// kamailio_tmx
	convertStatToMetric(completeStatMap, "tmx.active_transactions", "active", tmx, metricChannel, prometheus.GaugeValue)
	convertStatToMetric(completeStatMap, "tmx.inuse_transactions", "inuse", tmx, metricChannel, prometheus.GaugeValue)

	// kamailio_tmx_rpl_total
	convertStatToMetric(completeStatMap, "tmx.rpl_absorbed", "absorbed", tmx_rpl_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tmx.rpl_generated", "generated", tmx_rpl_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tmx.rpl_received", "received", tmx_rpl_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tmx.rpl_relayed", "relayed", tmx_rpl_total, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "tmx.rpl_sent", "sent", tmx_rpl_total, metricChannel, prometheus.CounterValue)

	// kamailio_dialog
	convertStatToMetric(completeStatMap, "dialog.active_dialogs", "active_dialogs", dialog, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "dialog.early_dialogs", "early_dialogs", dialog, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "dialog.expired_dialogs", "expired_dialogs", dialog, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "dialog.failed_dialogs", "failed_dialogs", dialog, metricChannel, prometheus.CounterValue)
	convertStatToMetric(completeStatMap, "dialog.processed_dialogs", "processed_dialogs", dialog, metricChannel, prometheus.CounterValue)
}

// Iterate all reported "stats" keys and find those with a prefix of "script."
// These values are user-defined and populated within the kamailio script.
// See https://www.kamailio.org/docs/modules/5.2.x/modules/statistics.html
func convertScriptedMetrics(data map[string]string, prom chan<- prometheus.Metric) {
	for k := range data {
		// k = "script.custom_total"
		if strings.HasPrefix(k, "script.") {
			// metricName = "custom_total"
			metricName := strings.TrimPrefix(k, "script.")
			metricName = strings.ToLower(metricName)
			var valueType prometheus.ValueType
			// deduce the metrics value type by following https://prometheus.io/docs/practices/naming/
			if strings.HasSuffix(k, "_total") || strings.HasSuffix(k, "_seconds") || strings.HasSuffix(k, "_bytes") {
				valueType = prometheus.CounterValue
			} else {
				valueType = prometheus.GaugeValue
			}
			// create a metric description on the fly
			description := prometheus.NewDesc("kamailio_"+metricName, "Scripted metric "+metricName, []string{}, nil)
			// and produce a metric
			convertStatToMetric(data, k, "", description, prom, valueType)
		}
	}
}

// convert a single "stat" value to a prometheus metric
// invalid "stat" paires are skipped but logged
func convertStatToMetric(completeStatMap map[string]string, statKey string, optionalLabelValue string, metricDescription *prometheus.Desc, metricChannel chan<- prometheus.Metric, valueType prometheus.ValueType) {
	// check wether we got a labelValue or not
	var labelValues []string
	if optionalLabelValue != "" {
		labelValues = []string{optionalLabelValue}
	} else {
		labelValues = []string{}
	}
	// get the stat-value ...
	if valueAsString, ok := completeStatMap[statKey]; ok {
		// ... convert it to a float
		if value, err := strconv.ParseFloat(valueAsString, 64); err == nil {
			// and produce a prometheus metric
			metric, err := prometheus.NewConstMetric(
				metricDescription,
				valueType,
				value,
				labelValues...,
			)
			if err == nil {
				// handover the metric to prometheus api
				metricChannel <- metric
			} else {
				// or skip and complain
				log.Warnf("Could not convert stat value [%s]: %s", statKey, err)
			}
		}
	} else {
		// skip stat values not found in completeStatMap
		// can happen if some kamailio modules are not loaded
		// and thus certain stat entries are not created
		log.Debugf("Skipping stat value [%s], it was not returned by kamailio", statKey)
	}
}
