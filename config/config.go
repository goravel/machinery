package config

import (
	"crypto/tls"
)

const (
	// DefaultResultsExpireIn is a default time used to expire task states and group metadata from the backend
	DefaultResultsExpireIn = 3600
)

// Config holds all configuration for our program
type Config struct {
	Broker                  string       `yaml:"broker"`
	Lock                    string       `yaml:"lock"`
	MultipleBrokerSeparator string       `yaml:"multiple_broker_separator"`
	DefaultQueue            string       `yaml:"default_queue"`
	ResultBackend           string       `yaml:"result_backend"`
	ResultsExpireIn         int          `yaml:"results_expire_in"`
	Redis                   *RedisConfig `yaml:"redis"`
	TLSConfig               *tls.Config
	// NoUnixSignals - when set disables signal handling in machinery
	NoUnixSignals bool `yaml:"no_unix_signals"`
}

// RedisConfig ...
type RedisConfig struct {
	// Maximum number of idle connections in the pool.
	// Default: 10
	MaxIdle int `yaml:"max_idle"`

	// Maximum number of connections allocated by the pool at a given time.
	// When zero, there is no limit on the number of connections in the pool.
	// Default: 100
	MaxActive int `yaml:"max_active"`

	// Close connections after remaining idle for this duration in seconds. If the value
	// is zero, then idle connections are not closed. Applications should set
	// the timeout to a value less than the server's timeout.
	// Default: 300
	IdleTimeout int `yaml:"max_idle_timeout"`

	// If Wait is true and the pool is at the MaxActive limit, then Get() waits
	// for a connection to be returned to the pool before returning.
	// Default: true
	Wait bool `yaml:"wait"`

	// ReadTimeout specifies the timeout in seconds for reading a single command reply.
	// Default: 15
	ReadTimeout int `yaml:"read_timeout"`

	// WriteTimeout specifies the timeout in seconds for writing a single command.
	// Default: 15
	WriteTimeout int `yaml:"write_timeout"`

	// ConnectTimeout specifies the timeout in seconds for connecting to the Redis server when
	// no DialNetDial option is specified.
	// Default: 15
	ConnectTimeout int `yaml:"connect_timeout"`

	// NormalTasksPollPeriod specifies the period in milliseconds when polling redis for normal tasks
	// Default: 1000
	NormalTasksPollPeriod int `yaml:"normal_tasks_poll_period"`

	// DelayedTasksPollPeriod specifies the period in milliseconds when polling redis for delayed tasks
	// Default: 20
	DelayedTasksPollPeriod int    `yaml:"delayed_tasks_poll_period"`
	DelayedTasksKey        string `yaml:"delayed_tasks_key"`

	// ClientName specifies the redis client name to be set when connecting to the Redis server
	ClientName string `yaml:"client_name"`

	// MasterName specifies a redis master name in order to configure a sentinel-backed redis FailoverClient
	MasterName string `yaml:"master_name"`

	// ClusterEnabled specifies whether cluster mode is enabled, regardless the number of addresses.
	// This helps create ClusterClient for Redis servers that enabled cluster mode with 1 node, or using AWS configuration endpoint
	ClusterEnabled bool `yaml:"cluster_enabled"`
}
