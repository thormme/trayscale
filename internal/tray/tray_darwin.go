//go:build darwin

package tray

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

void HideDock(void) {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}

void ShowDock(void) {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    [NSApp activateIgnoringOtherApps:YES];
}
*/
import "C"

import (
	"bytes"
	_ "embed"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"unique"

	"deedles.dev/trayscale/internal/tsutil"
	"fyne.io/systray"
)

var (
	//go:embed status-icon-active-template.png
	statusIconActiveData []byte

	//go:embed status-icon-inactive-template.png
	statusIconInactiveData []byte

	//go:embed status-icon-exit-node-template.png
	statusIconExitNodeData []byte

	selfHandle       = unique.Make("self")
	connToggleHandle = unique.Make("connToggle")
	exitToggleHandle = unique.Make("exitToggle")
	statusIconHandle = unique.Make("statusIcon")
)

type trayImpl struct {
	Callbacks

	m         sync.Mutex
	prev      map[unique.Handle[string]][]any
	prevBytes map[unique.Handle[string]][][]byte

	appStart  func()
	appClose  func()
	trayReady bool

	showItem       *systray.MenuItem
	connToggleItem *systray.MenuItem
	exitToggleItem *systray.MenuItem
	selfNodeItem   *systray.MenuItem
	quitItem       *systray.MenuItem
}

// New creates a new tray for the current platform
func New(cb Callbacks) Tray {
	return &trayImpl{Callbacks: cb}
}

func (t *trayImpl) Start(status *tsutil.IPNStatus) error {
	t.trayReady = false

	t.m.Lock()
	defer t.m.Unlock()

	onExit := func() {
		t.trayReady = false
		slog.Info("Tray exiting")
		t.close()
	}

	onReady := func() {
		systray.SetRemovalAllowed(true)
		systray.SetTemplateIcon(statusIconActiveData, statusIconActiveData)
		// systray.SetTitle("TS")

		t.showItem = systray.AddMenuItem("Show", "Show Trayscale")
		go func() {
			for range t.showItem.ClickedCh {
				t.OnShow()
			}
		}()
		systray.AddSeparator()
		t.connToggleItem = systray.AddMenuItemCheckbox("Connected", "Connect to tailscale", status.Online())
		go func() {
			for range t.connToggleItem.ClickedCh {
				t.OnConnToggle()
			}
		}()
		t.exitToggleItem = systray.AddMenuItemCheckbox("Exit Node Enabled", "Allow use of this device as an exit node", status.ExitNodeActive())
		go func() {
			for range t.exitToggleItem.ClickedCh {
				t.OnExitToggle()
			}
		}()
		t.selfNodeItem = systray.AddMenuItem(status.SelfAddr().String(), "Current Node IP")
		go func() {
			for range t.selfNodeItem.ClickedCh {
				t.OnSelfNode()
			}
		}()
		systray.AddSeparator()
		t.quitItem = systray.AddMenuItem("Quit", "Quit Trayscale (tailscale will remain running)")
		go func() {
			for range t.quitItem.ClickedCh {
				t.OnQuit()
			}
		}()

		t.trayReady = true

		t.update(status)
	}

	slog.Info("Starting loop")
	t.appStart, t.appClose = systray.RunWithExternalLoop(onReady, onExit)

	t.prev = make(map[unique.Handle[string]][]any)
	t.prevBytes = make(map[unique.Handle[string]][][]byte)

	t.appStart()

	return nil
}

func (t *trayImpl) Close() error {
	if t.appClose != nil {
		t.appClose()
	}
	return nil
}

func (t *trayImpl) HideDock() {
	C.HideDock()
}

func (t *trayImpl) ShowDock() {
	C.ShowDock()
}

func (t *trayImpl) close() error {
	if t == nil {
		return nil
	}

	t.m.Lock()
	defer t.m.Unlock()

	t.appClose = nil
	t.appStart = nil

	slog.Info("Quit")
	systray.Quit()
	t.prev = nil
	return nil
}

func (t *trayImpl) Update(s tsutil.Status) {
	if t == nil {
		return
	}

	status, ok := s.(*tsutil.IPNStatus)
	if !ok {
		return
	}

	t.m.Lock()
	defer t.m.Unlock()

	t.update(status)
}

func (t *trayImpl) dirty(key unique.Handle[string], vals ...any) bool {
	prev := t.prev[key]
	if slices.Equal(vals, prev) {
		return false
	}

	t.prev[key] = vals
	return true
}

func (t *trayImpl) dirtyBytes(key unique.Handle[string], vals ...[]byte) bool {
	prevBytesSlices := t.prevBytes[key]
	if len(prevBytesSlices) != len(vals) {
		t.prevBytes[key] = vals
		return true
	}
	for index, prevBytes := range prevBytesSlices {
		if !bytes.Equal(vals[index], prevBytes) {
			t.prevBytes[key] = vals
			return true
		}
	}
	return false
}

func (t *trayImpl) update(status *tsutil.IPNStatus) {
	if !t.trayReady {
		return
	}

	selfTitle, connected := selfTitle(status)
	connToggleLabel := connToggleText(status.Online())
	exitToggleLabel := exitToggleText(status)

	t.updateStatusIcon(status)

	if t.dirty(selfHandle, selfTitle, connected) {
		t.selfNodeItem.SetTitle(fmt.Sprintf("This machine: %v", selfTitle))
		if connected {
			t.selfNodeItem.Enable()
		} else {
			t.selfNodeItem.Disable()
		}
	}

	if t.dirty(connToggleHandle, connToggleLabel, status.Online()) {
		t.connToggleItem.SetTitle(connToggleLabel)
		if status.Online() {
			t.connToggleItem.Check()
		} else {
			t.connToggleItem.Uncheck()
		}
	}

	if t.dirty(exitToggleHandle, exitToggleLabel, connected, status.ExitNodeActive()) {
		t.exitToggleItem.SetTitle(exitToggleLabel)
		if connected {
			t.exitToggleItem.Enable()
		} else {
			t.exitToggleItem.Disable()
		}
		if status.ExitNodeActive() {
			t.exitToggleItem.Check()
		} else {
			t.exitToggleItem.Uncheck()
		}
	}
}

func (t *trayImpl) updateStatusIcon(status *tsutil.IPNStatus) {
	newIcon := statusIcon(status)
	if !t.dirtyBytes(statusIconHandle, newIcon) {
		return
	}

	systray.SetTemplateIcon(newIcon, newIcon)
}

func statusIcon(status *tsutil.IPNStatus) []byte {
	if !status.Online() {
		return statusIconInactiveData
	}
	if status.ExitNodeActive() {
		return statusIconExitNodeData
	}
	return statusIconActiveData
}

func selfTitle(status *tsutil.IPNStatus) (string, bool) {
	addr := status.SelfAddr()
	if !addr.IsValid() {
		return "Not connected", false
	}

	return fmt.Sprintf("%v (%v)", status.NetMap.SelfNode.DisplayName(true), addr), true
}

func connToggleText(online bool) string {
	if online {
		return "Disconnect"
	}

	return "Connect"
}

func exitToggleText(status *tsutil.IPNStatus) string {
	if status.ExitNodeActive() {
		return "Disable exit node"
	}

	return "Enable exit node"
}
