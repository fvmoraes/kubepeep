// Package config owns kubePeep's small, user-editable operational
// configuration. Security policy, filesystem locations, and Kubernetes
// selection intentionally do not belong to this schema.
package config

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	gingerconfig "github.com/fvmoraes/ginger/pkg/config"
	"gopkg.in/yaml.v3"
)

const (
	SchemaVersion       = 1
	MaxFileSize         = 64 * 1024
	DefaultPort         = 2748
	DefaultShutdownTime = 10 * time.Second
	OTelHTTPProtobuf    = "http/protobuf"
)

var secondsPattern = regexp.MustCompile(`^[1-9][0-9]*s$`)

// Duration is a deliberately narrow YAML duration. The v1 schema accepts only
// positive whole seconds so values such as "1m" cannot silently change
// meaning between implementations.
type Duration struct {
	time.Duration
}

func (d Duration) MarshalYAML() (any, error) {
	if d.Duration <= 0 || d.Duration%time.Second != 0 {
		return nil, fmt.Errorf("duration must contain positive whole seconds")
	}
	return fmt.Sprintf("%ds", int64(d.Duration/time.Second)), nil
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" || !secondsPattern.MatchString(node.Value) {
		return fmt.Errorf("duration must be a positive integer followed by s")
	}
	parsed, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration")
	}
	d.Duration = parsed
	return nil
}

// Config is the complete v1 user configuration.
type Config struct {
	Version       int                 `yaml:"version"`
	Server        ServerConfig        `yaml:"server"`
	Observability ObservabilityConfig `yaml:"observability"`
}

type ServerConfig struct {
	Port            *int     `yaml:"port"`
	OpenBrowser     bool     `yaml:"openBrowser"`
	ShutdownTimeout Duration `yaml:"shutdownTimeout"`
}

type ObservabilityConfig struct {
	OTel OTelConfig `yaml:"otel"`
}

type OTelConfig struct {
	Enabled  bool    `yaml:"enabled"`
	Endpoint *string `yaml:"endpoint"`
	Protocol string  `yaml:"protocol"`
	Insecure bool    `yaml:"insecure"`
}

// FlagOverrides represents only flags that override config.yaml. A nil Port
// means the flag was not present. --no-browser is one-way and can only disable
// browser opening.
type FlagOverrides struct {
	Port      *int
	NoBrowser bool
}

func Default() Config {
	return Config{
		Version: SchemaVersion,
		Server: ServerConfig{
			OpenBrowser:     true,
			ShutdownTimeout: Duration{Duration: DefaultShutdownTime},
		},
		Observability: ObservabilityConfig{OTel: OTelConfig{
			Protocol: OTelHTTPProtobuf,
		}},
	}
}

// ApplyFlags composes explicitly supplied CLI values over a validated file
// configuration without mutating the input value.
func ApplyFlags(base Config, flags FlagOverrides) (Config, error) {
	effective := base
	if base.Server.Port != nil {
		port := *base.Server.Port
		effective.Server.Port = &port
	}
	if flags.Port != nil {
		port := *flags.Port
		effective.Server.Port = &port
	}
	if flags.NoBrowser {
		effective.Server.OpenBrowser = false
	}
	if err := effective.Validate(); err != nil {
		return Config{}, err
	}
	return effective, nil
}

// Validate checks all cross-field and bounded v1 rules without including
// configuration values in returned errors.
func (c Config) Validate() error {
	if c.Version != SchemaVersion {
		return fmt.Errorf("config: unsupported version")
	}
	if c.Server.Port != nil && (*c.Server.Port < 1024 || *c.Server.Port > 65535) {
		return fmt.Errorf("config: server.port must be between 1024 and 65535")
	}
	if c.Server.ShutdownTimeout.Duration < time.Second || c.Server.ShutdownTimeout.Duration > 30*time.Second || c.Server.ShutdownTimeout.Duration%time.Second != 0 {
		return fmt.Errorf("config: server.shutdownTimeout must be between 1s and 30s")
	}

	otel := c.Observability.OTel
	if otel.Protocol != OTelHTTPProtobuf {
		return fmt.Errorf("config: observability.otel.protocol is unsupported")
	}
	if !otel.Enabled {
		if otel.Endpoint != nil {
			return fmt.Errorf("config: observability.otel.endpoint must be null when disabled")
		}
		return nil
	}
	if otel.Endpoint == nil {
		return fmt.Errorf("config: observability.otel.endpoint is required when enabled")
	}
	if len(*otel.Endpoint) == 0 || len(*otel.Endpoint) > 2048 {
		return fmt.Errorf("config: observability.otel.endpoint has an invalid length")
	}
	endpoint, err := url.Parse(*otel.Endpoint)
	if err != nil || !endpoint.IsAbs() || endpoint.Host == "" || endpoint.Opaque != "" {
		return fmt.Errorf("config: observability.otel.endpoint must be an absolute HTTP(S) URL")
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return fmt.Errorf("config: observability.otel.endpoint must be an absolute HTTP(S) URL")
	}
	if endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" {
		return fmt.Errorf("config: observability.otel.endpoint cannot contain userinfo, query, or fragment")
	}
	if endpoint.Scheme == "http" {
		if !otel.Insecure || !isLoopbackHost(endpoint.Hostname()) {
			return fmt.Errorf("config: HTTP observability endpoint requires loopback and insecure=true")
		}
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ToGinger maps validated product settings to Ginger's supported bootstrap
// struct. Host and pool settings are policy, not user configuration.
func (c Config) ToGinger(databasePath string) gingerconfig.Config {
	port := DefaultPort
	if c.Server.Port != nil {
		port = *c.Server.Port
	}
	return gingerconfig.Config{
		App: gingerconfig.AppConfig{
			Name:    "kubePeep",
			Env:     "local",
			Version: "dev",
		},
		HTTP: gingerconfig.HTTPConfig{
			Host:            "127.0.0.1",
			Port:            port,
			ShutdownTimeout: int(c.Server.ShutdownTimeout.Duration / time.Second),
		},
		Database: gingerconfig.DatabaseConfig{
			Driver:  "sqlite",
			DSN:     databasePath,
			MaxOpen: 4,
			MaxIdle: 4,
		},
		Log: gingerconfig.LogConfig{Level: "info", Format: "json"},
	}
}
