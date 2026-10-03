package metrics

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	dbTotalConnsDesc = prometheus.NewDesc(
		"vitalwatch_db_pool_total_conns",
		"Total number of connections in the database pool.",
		nil, nil,
	)
	dbAcquiredConnsDesc = prometheus.NewDesc(
		"vitalwatch_db_pool_acquired_conns",
		"Number of database connections currently acquired/in use.",
		nil, nil,
	)
	dbIdleConnsDesc = prometheus.NewDesc(
		"vitalwatch_db_pool_idle_conns",
		"Number of idle database connections in the pool.",
		nil, nil,
	)
	dbMaxConnsDesc = prometheus.NewDesc(
		"vitalwatch_db_pool_max_conns",
		"Maximum number of connections allowed in the pool.",
		nil, nil,
	)
	dbEmptyAcquireCountDesc = prometheus.NewDesc(
		"vitalwatch_db_pool_empty_acquires_total",
		"Total number of times a connection was acquired when the pool was empty.",
		nil, nil,
	)
)

// DBPoolCollector implements prometheus.Collector for pgxpool metrics.
type DBPoolCollector struct {
	pool *pgxpool.Pool
}

// NewDBPoolCollector creates a new collector for the given pgxpool.
func NewDBPoolCollector(pool *pgxpool.Pool) *DBPoolCollector {
	return &DBPoolCollector{pool: pool}
}

// Describe implements prometheus.Collector.
func (c *DBPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- dbTotalConnsDesc
	ch <- dbAcquiredConnsDesc
	ch <- dbIdleConnsDesc
	ch <- dbMaxConnsDesc
	ch <- dbEmptyAcquireCountDesc
}

// Collect implements prometheus.Collector.
func (c *DBPoolCollector) Collect(ch chan<- prometheus.Metric) {
	if c.pool == nil {
		return
	}
	stat := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(dbTotalConnsDesc, prometheus.GaugeValue, float64(stat.TotalConns()))
	ch <- prometheus.MustNewConstMetric(dbAcquiredConnsDesc, prometheus.GaugeValue, float64(stat.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(dbIdleConnsDesc, prometheus.GaugeValue, float64(stat.IdleConns()))
	ch <- prometheus.MustNewConstMetric(dbMaxConnsDesc, prometheus.GaugeValue, float64(stat.MaxConns()))
	ch <- prometheus.MustNewConstMetric(dbEmptyAcquireCountDesc, prometheus.CounterValue, float64(stat.EmptyAcquireCount()))
}

// Register registers the DBPoolCollector with the default Prometheus registry.
func Register(pool *pgxpool.Pool) {
	prometheus.MustRegister(NewDBPoolCollector(pool))
}

