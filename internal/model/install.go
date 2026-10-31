package model

import "context"

type Target struct {
	Version       string
	Loader        string
	LoaderVersion string
}

type Options struct {
	Download               string
	MaxConnections         int
	MaxRetries             int
	Argsment               string
	FollowModpackFilenames bool
	EnableModClassifier    bool
}

type InstallRequest struct {
	Root string
	Target
	Options
}

// FileTask describes a verified download, not an executable workflow step.
type FileTask struct {
	Path        string
	URLs        []string
	Hashes      map[string]string
	NameFromURL bool
}

type InstallPlan struct {
	Target Target
	Files  []FileTask
}

type Pack interface {
	Target() Target
	Name() string
	Prepare(context.Context, InstallRequest) error
	Close() error
}

type InstallResult struct {
	Target   Target
	PackName string
	Warnings []string
}
