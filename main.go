// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cemir/gomemcache/memcache"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	namespace           = "memcached"
	subsystemLruCrawler = "lru_crawler"
	subsystemSlab       = "slab"
)

// Exporter collects metrics from a memcached server.
type Exporter struct {
	address string
	timeout time.Duration

	up                       *prometheus.Desc
	uptime                   *prometheus.Desc
	version                  *prometheus.Desc
	bytesRead                *prometheus.Desc
	bytesWritten             *prometheus.Desc
	currentConnections       *prometheus.Desc
	maxConnections           *prometheus.Desc
	connectionsTotal         *prometheus.Desc
	connsYieldedTotal        *prometheus.Desc
	listenerDisabledTotal    *prometheus.Desc
	currentBytes             *prometheus.Desc
	limitBytes               *prometheus.Desc
	commands                 *prometheus.Desc
	items                    *prometheus.Desc
	itemsTotal               *prometheus.Desc
	evictions                *prometheus.Desc
	reclaimed                *prometheus.Desc
	lruCrawlerEnabled        *prometheus.Desc
	lruCrawlerSleep          *prometheus.Desc
	lruCrawlerMaxItems       *prometheus.Desc
	lruMaintainerThread      *prometheus.Desc
	lruHotPercent            *prometheus.Desc
	lruWarmPercent           *prometheus.Desc
	lruHotMaxAgeFactor       *prometheus.Desc
	lruWarmMaxAgeFactor      *prometheus.Desc
	lruCrawlerStarts         *prometheus.Desc
	lruCrawlerReclaimed      *prometheus.Desc
	lruCrawlerItemsChecked   *prometheus.Desc
	lruCrawlerMovesToCold    *prometheus.Desc
	lruCrawlerMovesToWarm    *prometheus.Desc
	lruCrawlerMovesWithinLru *prometheus.Desc
	malloced                 *prometheus.Desc
	itemsNumber              *prometheus.Desc
	itemsAge                 *prometheus.Desc
	itemsCrawlerReclaimed    *prometheus.Desc
	itemsEvicted             *prometheus.Desc
	itemsEvictedNonzero      *prometheus.Desc
	itemsEvictedTime         *prometheus.Desc
	itemsEvictedUnfetched    *prometheus.Desc
	itemsExpiredUnfetched    *prometheus.Desc
	itemsOutofmemory         *prometheus.Desc
	itemsReclaimed           *prometheus.Desc
	itemsTailrepairs         *prometheus.Desc
	itemsMovesToCold         *prometheus.Desc
	itemsMovesToWarm         *prometheus.Desc
	itemsMovesWithinLru      *prometheus.Desc
	slabsChunkSize           *prometheus.Desc
	slabsChunksPerPage       *prometheus.Desc
	slabsCurrentPages        *prometheus.Desc
	slabsCurrentChunks       *prometheus.Desc
	slabsChunksUsed          *prometheus.Desc
	slabsChunksFree          *prometheus.Desc
	slabsChunksFreeEnd       *prometheus.Desc
	slabsMemRequested        *prometheus.Desc
	slabsCommands            *prometheus.Desc
}

// NewExporter returns an initialized exporter.
func NewExporter(server string, timeout time.Duration) *Exporter {
	return &Exporter{
		address: server,
		timeout: timeout,
		up: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "up"),
			"Could the memcached server be reached.",
			nil,
			nil,
		),
		uptime: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "uptime_seconds"),
			"Number of seconds since the server started.",
			nil,
			nil,
		),
		version: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "version"),
			"The version of this memcached server.",
			[]string{"version"},
			nil,
		),
		bytesRead: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "read_bytes_total"),
			"Total number of bytes read by this server from network.",
			nil,
			nil,
		),
		bytesWritten: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "written_bytes_total"),
			"Total number of bytes sent by this server to network.",
			nil,
			nil,
		),
		currentConnections: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "current_connections"),
			"Current number of open connections.",
			nil,
			nil,
		),
		maxConnections: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "max_connections"),
			"Maximum number of clients allowed.",
			nil,
			nil,
		),
		connectionsTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "connections_total"),
			"Total number of connections opened since the server started running.",
			nil,
			nil,
		),
		connsYieldedTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "connections_yielded_total"),
			"Total number of connections yielded running due to hitting the memcached's -R limit.",
			nil,
			nil,
		),
		listenerDisabledTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "connections_listener_disabled_total"),
			"Number of times that memcached has hit its connections limit and disabled its listener.",
			nil,
			nil,
		),
		currentBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "current_bytes"),
			"Current number of bytes used to store items.",
			nil,
			nil,
		),
		limitBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "limit_bytes"),
			"Number of bytes this server is allowed to use for storage.",
			nil,
			nil,
		),
		commands: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "commands_total"),
			"Total number of all requests broken down by command (get, set, etc.) and status.",
			[]string{"command", "status"},
			nil,
		),
		items: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "current_items"),
			"Current number of items stored by this instance.",
			nil,
			nil,
		),
		itemsTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "items_total"),
			"Total number of items stored during the life of this instance.",
			nil,
			nil,
		),
		evictions: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "items_evicted_total"),
			"Total number of valid items removed from cache to free memory for new items.",
			nil,
			nil,
		),
		reclaimed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "items_reclaimed_total"),
			"Total number of times an entry was stored using memory from an expired entry.",
			nil,
			nil,
		),
		lruCrawlerEnabled: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "enabled"),
			"Whether the LRU crawler is enabled.",
			nil,
			nil,
		),
		lruCrawlerSleep: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "sleep"),
			"Microseconds to sleep between LRU crawls.",
			nil,
			nil,
		),
		lruCrawlerMaxItems: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "to_crawl"),
			"Max items to crawl per slab per run.",
			nil,
			nil,
		),
		lruMaintainerThread: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "maintainer_thread"),
			"Split LRU mode and background threads.",
			nil,
			nil,
		),
		lruHotPercent: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "hot_percent"),
			"Percent of slab memory reserved for HOT LRU.",
			nil,
			nil,
		),
		lruWarmPercent: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "warm_percent"),
			"Percent of slab memory reserved for WARM LRU.",
			nil,
			nil,
		),
		lruHotMaxAgeFactor: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "hot_max_factor"),
			"Set idle age of HOT LRU to COLD age * this",
			nil,
			nil,
		),
		lruWarmMaxAgeFactor: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "warm_max_factor"),
			"Set idle age of WARM LRU to COLD age * this",
			nil,
			nil,
		),
		lruCrawlerStarts: prometheus.NewDesc(
			prometheus.BuildFQName("namespace", subsystemLruCrawler, "starts"),
			"Times an LRU crawler was started.",
			nil,
			nil,
		),
		lruCrawlerReclaimed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "reclaimed_total"),
			"Total items freed by LRU Crawler.",
			nil,
			nil,
		),
		lruCrawlerItemsChecked: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "items_checked_total"),
			"Total items examined by LRU Crawler.",
			nil,
			nil,
		),
		lruCrawlerMovesToCold: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "moves_to_cold_total"),
			"Total number of items moved from HOT/WARM to COLD LRU's.",
			nil,
			nil,
		),
		lruCrawlerMovesToWarm: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "moves_to_warm_total"),
			"Total number of items moved from COLD to WARM LRU.",
			nil,
			nil,
		),
		lruCrawlerMovesWithinLru: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemLruCrawler, "moves_within_lru_total"),
			"Total number of items reshuffled within HOT or WARM LRU's.",
			nil,
			nil,
		),
		malloced: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "malloced_bytes"),
			"Number of bytes of memory allocated to slab pages.",
			nil,
			nil,
		),
		itemsNumber: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "current_items"),
			"Number of items currently stored in this slab class.",
			[]string{"slab"},
			nil,
		),
		itemsAge: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_age_seconds"),
			"Number of seconds the oldest item has been in the slab class.",
			[]string{"slab"},
			nil,
		),
		itemsCrawlerReclaimed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_crawler_reclaimed_total"),
			"Number of items freed by the LRU Crawler.",
			[]string{"slab"},
			nil,
		),
		itemsEvicted: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_evicted_total"),
			"Total number of times an item had to be evicted from the LRU before it expired.",
			[]string{"slab"},
			nil,
		),
		itemsEvictedNonzero: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_evicted_nonzero_total"),
			"Total number of times an item which had an explicit expire time set had to be evicted from the LRU before it expired.",
			[]string{"slab"},
			nil,
		),
		itemsEvictedTime: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_evicted_time_seconds"),
			"Seconds since the last access for the most recent item evicted from this class.",
			[]string{"slab"},
			nil,
		),
		itemsEvictedUnfetched: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_evicted_unfetched_total"),
			"Total nmber of items evicted and never fetched.",
			[]string{"slab"},
			nil,
		),
		itemsExpiredUnfetched: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_expired_unfetched_total"),
			"Total number of valid items evicted from the LRU which were never touched after being set.",
			[]string{"slab"},
			nil,
		),
		itemsOutofmemory: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_outofmemory_total"),
			"Total number of items for this slab class that have triggered an out of memory error.",
			[]string{"slab"},
			nil,
		),
		itemsReclaimed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_reclaimed_total"),
			"Total number of items reclaimed.",
			[]string{"slab"},
			nil,
		),
		itemsTailrepairs: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_tailrepairs_total"),
			"Total number of times the entries for a particular ID need repairing.",
			[]string{"slab"},
			nil,
		),
		itemsMovesToCold: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_moves_to_cold"),
			"Number of items moved from HOT or WARM into COLD.",
			[]string{"slab"},
			nil,
		),
		itemsMovesToWarm: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_moves_to_warm"),
			"Number of items moves from COLD into WARM.",
			[]string{"slab"},
			nil,
		),
		itemsMovesWithinLru: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "items_moves_within_lru"),
			"Number of times active items were bumped within HOT or WARM.",
			[]string{"slab"},
			nil,
		),
		slabsChunkSize: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "chunk_size_bytes"),
			"Number of bytes allocated to each chunk within this slab class.",
			[]string{"slab"},
			nil,
		),
		slabsChunksPerPage: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "chunks_per_page"),
			"Number of chunks within a single page for this slab class.",
			[]string{"slab"},
			nil,
		),
		slabsCurrentPages: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "current_pages"),
			"Number of pages allocated to this slab class.",
			[]string{"slab"},
			nil,
		),
		slabsCurrentChunks: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "current_chunks"),
			"Number of chunks allocated to this slab class.",
			[]string{"slab"},
			nil,
		),
		slabsChunksUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "chunks_used"),
			"Number of chunks allocated to an item.",
			[]string{"slab"},
			nil,
		),
		slabsChunksFree: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "chunks_free"),
			"Number of chunks not yet allocated items.",
			[]string{"slab"},
			nil,
		),
		slabsChunksFreeEnd: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "chunks_free_end"),
			"Number of free chunks at the end of the last allocated page.",
			[]string{"slab"},
			nil,
		),
		slabsMemRequested: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "mem_requested_bytes"),
			"Number of bytes of memory actual items take up within a slab.",
			[]string{"slab"},
			nil,
		),
		slabsCommands: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystemSlab, "commands_total"),
			"Total number of all requests broken down by command (get, set, etc.) and status per slab.",
			[]string{"slab", "command", "status"},
			nil,
		),
	}
}

// Describe describes all the metrics exported by the memcached exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.up
	ch <- e.uptime
	ch <- e.version
	ch <- e.bytesRead
	ch <- e.bytesWritten
	ch <- e.currentConnections
	ch <- e.maxConnections
	ch <- e.connectionsTotal
	ch <- e.connsYieldedTotal
	ch <- e.listenerDisabledTotal
	ch <- e.currentBytes
	ch <- e.limitBytes
	ch <- e.commands
	ch <- e.items
	ch <- e.itemsTotal
	ch <- e.evictions
	ch <- e.reclaimed
	ch <- e.lruCrawlerEnabled
	ch <- e.lruCrawlerSleep
	ch <- e.lruCrawlerMaxItems
	ch <- e.lruMaintainerThread
	ch <- e.lruHotPercent
	ch <- e.lruWarmPercent
	ch <- e.lruHotMaxAgeFactor
	ch <- e.lruWarmMaxAgeFactor
	ch <- e.lruCrawlerStarts
	ch <- e.lruCrawlerReclaimed
	ch <- e.lruCrawlerItemsChecked
	ch <- e.lruCrawlerMovesToCold
	ch <- e.lruCrawlerMovesToWarm
	ch <- e.lruCrawlerMovesWithinLru
	ch <- e.malloced
	ch <- e.itemsNumber
	ch <- e.itemsAge
	ch <- e.itemsCrawlerReclaimed
	ch <- e.itemsEvicted
	ch <- e.itemsEvictedNonzero
	ch <- e.itemsEvictedTime
	ch <- e.itemsEvictedUnfetched
	ch <- e.itemsExpiredUnfetched
	ch <- e.itemsOutofmemory
	ch <- e.itemsReclaimed
	ch <- e.itemsTailrepairs
	ch <- e.itemsExpiredUnfetched
	ch <- e.itemsMovesToCold
	ch <- e.itemsMovesToWarm
	ch <- e.itemsMovesWithinLru
	ch <- e.slabsChunkSize
	ch <- e.slabsChunksPerPage
	ch <- e.slabsCurrentPages
	ch <- e.slabsCurrentChunks
	ch <- e.slabsChunksUsed
	ch <- e.slabsChunksFree
	ch <- e.slabsChunksFreeEnd
	ch <- e.slabsMemRequested
	ch <- e.slabsCommands
}

// Collect fetches the statistics from the configured memcached server, and
// delivers them as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	c, err := memcache.New(e.address)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 0)
		log.Errorf("Failed to connect to memcached: %s", err)
		return
	}
	c.Timeout = e.timeout

	stats, err := c.Stats()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 0)
		log.Errorf("Failed to collect stats from memcached: %s", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 1)

	// TODO(ts): Clean up and consolidate metric mappings.
	itemsMetrics := map[string]*prometheus.Desc{
		"crawler_reclaimed": e.itemsCrawlerReclaimed,
		"evicted":           e.itemsEvicted,
		"evicted_nonzero":   e.itemsEvictedNonzero,
		"evicted_time":      e.itemsEvictedTime,
		"evicted_unfetched": e.itemsEvictedUnfetched,
		"expired_unfetched": e.itemsExpiredUnfetched,
		"outofmemory":       e.itemsOutofmemory,
		"reclaimed":         e.itemsReclaimed,
		"tailrepairs":       e.itemsTailrepairs,
		"moves_to_cold":     e.itemsMovesToCold,
		"moves_to_warm":     e.itemsMovesToWarm,
		"moves_within_lru":  e.itemsMovesWithinLru,
	}

	for _, t := range stats {
		s := t.Stats
		ch <- prometheus.MustNewConstMetric(e.uptime, prometheus.CounterValue, parse(s, "uptime"))
		ch <- prometheus.MustNewConstMetric(e.version, prometheus.GaugeValue, 1, s["version"])

		for _, op := range []string{"get", "delete", "incr", "decr", "cas", "touch"} {
			ch <- prometheus.MustNewConstMetric(e.commands, prometheus.CounterValue, parse(s, op+"_hits"), op, "hit")
			ch <- prometheus.MustNewConstMetric(e.commands, prometheus.CounterValue, parse(s, op+"_misses"), op, "miss")
		}
		ch <- prometheus.MustNewConstMetric(e.commands, prometheus.CounterValue, parse(s, "cas_badval"), "cas", "badval")
		ch <- prometheus.MustNewConstMetric(e.commands, prometheus.CounterValue, parse(s, "cmd_flush"), "flush", "hit")

		// memcached includes cas operations again in cmd_set.
		set := math.NaN()
		if setCmd, err := strconv.ParseFloat(s["cmd_set"], 64); err == nil {
			if cas, casErr := sum(s, "cas_misses", "cas_hits", "cas_badval"); casErr == nil {
				set = setCmd - cas
			} else {
				log.Errorf("Failed to parse cas: %s", casErr)
			}
		} else {
			log.Errorf("Failed to parse set %q: %s", s["cmd_set"], err)
		}
		ch <- prometheus.MustNewConstMetric(e.commands, prometheus.CounterValue, set, "set", "hit")

		ch <- prometheus.MustNewConstMetric(e.currentBytes, prometheus.GaugeValue, parse(s, "bytes"))
		ch <- prometheus.MustNewConstMetric(e.limitBytes, prometheus.GaugeValue, parse(s, "limit_maxbytes"))
		ch <- prometheus.MustNewConstMetric(e.items, prometheus.GaugeValue, parse(s, "curr_items"))
		ch <- prometheus.MustNewConstMetric(e.itemsTotal, prometheus.CounterValue, parse(s, "total_items"))

		ch <- prometheus.MustNewConstMetric(e.bytesRead, prometheus.CounterValue, parse(s, "bytes_read"))
		ch <- prometheus.MustNewConstMetric(e.bytesWritten, prometheus.CounterValue, parse(s, "bytes_written"))

		ch <- prometheus.MustNewConstMetric(e.currentConnections, prometheus.GaugeValue, parse(s, "curr_connections"))
		ch <- prometheus.MustNewConstMetric(e.connectionsTotal, prometheus.CounterValue, parse(s, "total_connections"))
		ch <- prometheus.MustNewConstMetric(e.connsYieldedTotal, prometheus.CounterValue, parse(s, "conn_yields"))
		ch <- prometheus.MustNewConstMetric(e.listenerDisabledTotal, prometheus.CounterValue, parse(s, "listen_disabled_num"))

		ch <- prometheus.MustNewConstMetric(e.evictions, prometheus.CounterValue, parse(s, "evictions"))
		ch <- prometheus.MustNewConstMetric(e.reclaimed, prometheus.CounterValue, parse(s, "reclaimed"))

		ch <- prometheus.MustNewConstMetric(e.lruCrawlerStarts, prometheus.UntypedValue, parse(s, "lru_crawler_starts"))
		ch <- prometheus.MustNewConstMetric(e.lruCrawlerItemsChecked, prometheus.CounterValue, parse(s, "crawler_items_checked"))
		ch <- prometheus.MustNewConstMetric(e.lruCrawlerReclaimed, prometheus.CounterValue, parse(s, "crawler_reclaimed"))
		ch <- prometheus.MustNewConstMetric(e.lruCrawlerMovesToCold, prometheus.CounterValue, parse(s, "moves_to_cold"))
		ch <- prometheus.MustNewConstMetric(e.lruCrawlerMovesToWarm, prometheus.CounterValue, parse(s, "moves_to_warm"))
		ch <- prometheus.MustNewConstMetric(e.lruCrawlerMovesWithinLru, prometheus.CounterValue, parse(s, "moves_within_lru"))

		ch <- prometheus.MustNewConstMetric(e.malloced, prometheus.GaugeValue, parse(s, "total_malloced"))

		for slab, u := range t.Items {
			slab := strconv.Itoa(slab)
			ch <- prometheus.MustNewConstMetric(e.itemsNumber, prometheus.GaugeValue, parse(u, "number"), slab)
			ch <- prometheus.MustNewConstMetric(e.itemsAge, prometheus.GaugeValue, parse(u, "age"), slab)
			for m, d := range itemsMetrics {
				if _, ok := u[m]; !ok {
					continue
				}
				ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, parse(u, m), slab)
			}
		}

		for slab, v := range t.Slabs {
			slab := strconv.Itoa(slab)

			for _, op := range []string{"get", "delete", "incr", "decr", "cas", "touch"} {
				ch <- prometheus.MustNewConstMetric(e.slabsCommands, prometheus.CounterValue, parse(v, op+"_hits"), slab, op, "hit")
			}
			ch <- prometheus.MustNewConstMetric(e.slabsCommands, prometheus.CounterValue, parse(v, "cas_badval"), slab, "cas", "badval")

			slabSet := math.NaN()
			if slabSetCmd, err := strconv.ParseFloat(v["cmd_set"], 64); err == nil {
				if slabCas, slabCasErr := sum(v, "cas_hits", "cas_badval"); slabCasErr == nil {
					slabSet = slabSetCmd - slabCas
				} else {
					log.Errorf("Failed to parse cas: %s", slabCasErr)
				}
			} else {
				log.Errorf("Failed to parse set %q: %s", v["cmd_set"], err)
			}
			ch <- prometheus.MustNewConstMetric(e.slabsCommands, prometheus.CounterValue, slabSet, slab, "set", "hit")

			ch <- prometheus.MustNewConstMetric(e.slabsChunkSize, prometheus.GaugeValue, parse(v, "chunk_size"), slab)
			ch <- prometheus.MustNewConstMetric(e.slabsChunksPerPage, prometheus.GaugeValue, parse(v, "chunks_per_page"), slab)
			ch <- prometheus.MustNewConstMetric(e.slabsCurrentPages, prometheus.GaugeValue, parse(v, "total_pages"), slab)
			ch <- prometheus.MustNewConstMetric(e.slabsCurrentChunks, prometheus.GaugeValue, parse(v, "total_chunks"), slab)
			ch <- prometheus.MustNewConstMetric(e.slabsChunksUsed, prometheus.GaugeValue, parse(v, "used_chunks"), slab)
			ch <- prometheus.MustNewConstMetric(e.slabsChunksFree, prometheus.GaugeValue, parse(v, "free_chunks"), slab)
			ch <- prometheus.MustNewConstMetric(e.slabsChunksFreeEnd, prometheus.GaugeValue, parse(v, "free_chunks_end"), slab)
			ch <- prometheus.MustNewConstMetric(e.slabsMemRequested, prometheus.GaugeValue, parse(v, "mem_requested"), slab)
		}
	}

	statsSettings, err := c.StatsSettings()
	if err != nil {
		log.Errorf("Could not query stats settings: %s", err)
	}
	for _, settings := range statsSettings {
		ch <- prometheus.MustNewConstMetric(e.maxConnections, prometheus.GaugeValue, parse(settings, "maxconns"))
		ch <- prometheus.MustNewConstMetric(e.lruCrawlerEnabled, prometheus.GaugeValue, parseBool(settings, "lru_crawler"))
		ch <- prometheus.MustNewConstMetric(e.lruCrawlerSleep, prometheus.GaugeValue, parse(settings, "lru_crawler_sleep"))
		ch <- prometheus.MustNewConstMetric(e.lruCrawlerMaxItems, prometheus.GaugeValue, parse(settings, "lru_crawler_tocrawl"))
		ch <- prometheus.MustNewConstMetric(e.lruMaintainerThread, prometheus.GaugeValue, parseBool(settings, "lru_maintainer_thread"))
		ch <- prometheus.MustNewConstMetric(e.lruHotPercent, prometheus.GaugeValue, parse(settings, "hot_lru_pct"))
		ch <- prometheus.MustNewConstMetric(e.lruWarmPercent, prometheus.GaugeValue, parse(settings, "warm_lru_pct"))
		ch <- prometheus.MustNewConstMetric(e.lruHotMaxAgeFactor, prometheus.GaugeValue, parse(settings, "hot_max_factor"))
		ch <- prometheus.MustNewConstMetric(e.lruWarmMaxAgeFactor, prometheus.GaugeValue, parse(settings, "warm_max_factor"))
	}
}

func parse(stats map[string]string, key string) float64 {
	v, err := strconv.ParseFloat(stats[key], 64)
	if err != nil {
		log.Errorf("Failed to parse %s %q: %s", key, stats[key], err)
		v = math.NaN()
	}
	return v
}

func parseBool(stats map[string]string, key string) float64 {
	switch stats[key] {
	case "yes":
		return 1
	case "no":
		return 0
	default:
		log.Errorf("Failed parse %s %q", key, stats[key])
		return math.NaN()
	}
}

func sum(stats map[string]string, keys ...string) (float64, error) {
	s := 0.
	for _, key := range keys {
		v, err := strconv.ParseFloat(stats[key], 64)
		if err != nil {
			return math.NaN(), err
		}
		s += v
	}
	return s, nil
}

func main() {
	var (
		address       = kingpin.Flag("memcached.address", "Memcached server address.").Default("localhost:11211").String()
		timeout       = kingpin.Flag("memcached.timeout", "memcached connect timeout.").Default("1s").Duration()
		pidFile       = kingpin.Flag("memcached.pid-file", "Optional path to a file containing the memcached PID for additional metrics.").Default("").String()
		unixSocket    = kingpin.Flag("memcached.unix-socket", "Optional path to the unix socket file.").Default("").String()
		listenAddress = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry.").Default(":9150").String()
		metricsPath   = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
	)
	log.AddFlags(kingpin.CommandLine)
	kingpin.Version(version.Print("memcached_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	log.Infoln("Starting memcached_exporter", version.Info())
	log.Infoln("Build context", version.BuildContext())

	if *unixSocket != "" {
		fmt.Printf("socket set\n")
	}

	prometheus.MustRegister(NewExporter(*address, *timeout))
	if *pidFile != "" {
		procExporter := prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{
			PidFn: func() (int, error) {
				content, err := ioutil.ReadFile(*pidFile)
				if err != nil {
					return 0, fmt.Errorf("can't read pid file %q: %s", *pidFile, err)
				}
				value, err := strconv.Atoi(strings.TrimSpace(string(content)))
				if err != nil {
					return 0, fmt.Errorf("can't parse pid file %q: %s", *pidFile, err)
				}
				return value, nil
			},
			Namespace: namespace,
		})
		prometheus.MustRegister(procExporter)
	}

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Memcached Exporter</title></head>
             <body>
             <h1>Memcached Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})
	log.Infoln("Starting HTTP server on", *listenAddress)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
