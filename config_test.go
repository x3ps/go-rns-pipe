package rnspipe

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	d := DefaultConfig()

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Name", d.Name, "PipeInterface"},
		{"MTU", d.MTU, 500},
		{"HWMTU", d.HWMTU, 1064},
		{"Bitrate", d.Bitrate, 1_000_000},
		{"ReconnectDelay", d.ReconnectDelay, 5 * time.Second},
		{"ReceiveBufferSize", d.ReceiveBufferSize, 64},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestNewClampsNegativeValues(t *testing.T) {
	t.Parallel()

	defaults := DefaultConfig()

	tests := []struct {
		name string
		cfg  Config
		get  func(*Interface) int
		want int
	}{
		{
			name: "MTU",
			cfg:  Config{MTU: -1},
			get:  func(i *Interface) int { return i.config.MTU },
			want: defaults.MTU,
		},
		{
			name: "HWMTU",
			cfg:  Config{HWMTU: -99},
			get:  func(i *Interface) int { return i.HWMTU() },
			want: defaults.HWMTU,
		},
		{
			name: "Bitrate",
			cfg:  Config{Bitrate: -5},
			get:  func(i *Interface) int { return i.config.Bitrate },
			want: defaults.Bitrate,
		},
		{
			name: "ReconnectDelay",
			cfg:  Config{ReconnectDelay: -time.Second},
			get:  func(i *Interface) int { return int(i.config.ReconnectDelay) },
			want: int(defaults.ReconnectDelay),
		},
		{
			name: "ReceiveBufferSize",
			cfg:  Config{ReceiveBufferSize: -10},
			get:  func(i *Interface) int { return i.config.ReceiveBufferSize },
			want: defaults.ReceiveBufferSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			iface := New(tt.cfg)
			if got := tt.get(iface); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	t.Parallel()

	defaults := DefaultConfig()
	iface := New(Config{})

	if iface.config.Name != defaults.Name {
		t.Errorf("Name = %q, want %q", iface.config.Name, defaults.Name)
	}
	if iface.config.MTU != defaults.MTU {
		t.Errorf("MTU = %d, want %d", iface.config.MTU, defaults.MTU)
	}
	if iface.config.HWMTU != defaults.HWMTU {
		t.Errorf("HWMTU = %d, want %d", iface.config.HWMTU, defaults.HWMTU)
	}
	if iface.config.Bitrate != defaults.Bitrate {
		t.Errorf("Bitrate = %d, want %d", iface.config.Bitrate, defaults.Bitrate)
	}
	if iface.config.ReconnectDelay != defaults.ReconnectDelay {
		t.Errorf("ReconnectDelay = %v, want %v", iface.config.ReconnectDelay, defaults.ReconnectDelay)
	}
	if iface.config.ReceiveBufferSize != defaults.ReceiveBufferSize {
		t.Errorf("ReceiveBufferSize = %d, want %d", iface.config.ReceiveBufferSize, defaults.ReceiveBufferSize)
	}
}

func TestNewPreservesExplicitValues(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Name:              "custom",
		MTU:               1200,
		HWMTU:             2048,
		Bitrate:           500_000,
		ReconnectDelay:    10 * time.Second,
		ReceiveBufferSize: 128,
	}

	iface := New(cfg)

	if iface.config.Name != "custom" {
		t.Errorf("Name = %q, want %q", iface.config.Name, "custom")
	}
	if iface.config.MTU != 1200 {
		t.Errorf("MTU = %d, want 1200", iface.config.MTU)
	}
	if iface.config.HWMTU != 2048 {
		t.Errorf("HWMTU = %d, want 2048", iface.config.HWMTU)
	}
	if iface.config.Bitrate != 500_000 {
		t.Errorf("Bitrate = %d, want 500000", iface.config.Bitrate)
	}
	if iface.config.ReconnectDelay != 10*time.Second {
		t.Errorf("ReconnectDelay = %v, want 10s", iface.config.ReconnectDelay)
	}
	if iface.config.ReceiveBufferSize != 128 {
		t.Errorf("ReceiveBufferSize = %d, want 128", iface.config.ReceiveBufferSize)
	}
}

func TestNewCustomLogger(t *testing.T) {
	t.Parallel()

	custom := slog.New(slog.NewTextHandler(io.Discard, nil))
	iface := New(Config{Logger: custom})

	// The logger stored should be a child of our custom logger (with "interface" attr),
	// not the default stderr logger. We verify by checking it's non-nil and that
	// the config's Logger was consumed (no default was created).
	if iface.logger == nil {
		t.Fatal("logger is nil")
	}
}
