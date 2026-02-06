package tray

import "deedles.dev/trayscale/internal/tsutil"

// Tray defines the interface for system tray implementations
type Tray interface {
	Start(status *tsutil.IPNStatus) error
	Close() error
	Update(s tsutil.Status)
}

// Callbacks holds the tray event handlers
type Callbacks struct {
	OnShow       func()
	OnConnToggle func()
	OnExitToggle func()
	OnSelfNode   func()
	OnQuit       func()
}
