package tray_macos

import (
	_ "embed"
	"fmt"
	"slices"
	"sync"
	"unique"

	"deedles.dev/trayscale/internal/tsutil"
	"fyne.io/systray"
)

var (
	//go:embed status-icon-active.png
	statusIconActiveData []byte

	//go:embed status-icon-inactive.png
	statusIconInactiveData []byte

	//go:embed status-icon-exit-node.png
	statusIconExitNodeData []byte

	selfHandle       = unique.Make("self")
	connToggleHandle = unique.Make("connToggle")
	exitToggleHandle = unique.Make("exitToggle")
	statusIconHandle = unique.Make("statusIcon")
)

type Tray struct {
	OnShow       func()
	OnConnToggle func()
	OnExitToggle func()
	OnSelfNode   func()
	OnQuit       func()

	m    sync.Mutex
	prev map[unique.Handle[string]][]any

	appStart  func()
	appClose  func()
	trayReady bool

	showItem       *systray.MenuItem
	connToggleItem *systray.MenuItem
	exitToggleItem *systray.MenuItem
	selfNodeItem   *systray.MenuItem
	quitItem       *systray.MenuItem
}

func (t *Tray) Start(status *tsutil.IPNStatus) error {
	t.trayReady = false

	t.m.Lock()
	defer t.m.Unlock()

	onExit := func() {
		t.trayReady = false
		fmt.Println("Exit")
	}

	onReady := func() {
		systray.SetTitle("Trayscale")

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
			for range t.connToggleItem.ClickedCh {
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

		t.appStart()

	}

	fmt.Println("Starting loop")
	t.appStart, t.appClose = systray.RunWithExternalLoop(onReady, onExit)

	// item, err := tray.New(
	// 	tray.ItemID("dev.deedles.Trayscale"),
	// 	tray.ItemTitle("Trayscale"),
	// 	tray.ItemHandler(tray.ActivateHandler(func(x, y int) error {
	// 		t.OnShow()
	// 		return nil
	// 	})),
	// )
	// if err != nil {
	// 	return err
	// }
	// t.item = item
	t.prev = make(map[unique.Handle[string]][]any)

	return nil
}

func (t *Tray) Close() error {
	if t == nil {
		return nil
	}

	t.m.Lock()
	defer t.m.Unlock()

	// systray.close()
	t.prev = nil
	return nil
}

func (t *Tray) Update(s tsutil.Status) {
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

func (t *Tray) dirty(key unique.Handle[string], vals ...any) bool {
	prev := t.prev[key]
	if slices.Equal(vals, prev) {
		return false
	}

	t.prev[key] = vals
	return true
}

func (t *Tray) update(status *tsutil.IPNStatus) {
	if t.trayReady == false {
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

	if t.dirty(connToggleHandle, connToggleLabel) {
		t.connToggleItem.SetTitle(connToggleLabel)
	}

	if t.dirty(exitToggleHandle, exitToggleLabel, connected) {
		t.exitToggleItem.SetTitle(exitToggleLabel)
		if connected {
			t.exitToggleItem.Enable()
		} else {
			t.exitToggleItem.Disable()
		}
	}
}

func (t *Tray) updateStatusIcon(status *tsutil.IPNStatus) {
	newIcon := statusIcon(status)
	if !t.dirty(statusIconHandle, newIcon) {
		return
	}

	systray.SetIcon(newIcon)
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
		// TODO: Show some actual information about the current exit node?
		return "Disable exit node"
	}

	return "Enable exit node"
}
