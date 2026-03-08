package modcheck

import "time"

type Config struct {
	EnableModrinth bool
	EnableMixin    bool
	EnableHash     bool
	Timeout        time.Duration
}

var DefaultConfig = Config{
	EnableModrinth: true,
	EnableMixin:    true,
	EnableHash:     true,
	Timeout:        30 * time.Second,
}

type MixinFile struct {
	Name string
	Data string
}

type ModFile struct {
	Path       string
	Name       string
	Hash       string
	Mixins     []MixinFile
	ModIDs     []string
	ProjectIDs []string
}
