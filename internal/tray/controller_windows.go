//go:build windows

package tray

import (
	"context"
	_ "embed"
	"sync"

	"fyne.io/systray"
)

//go:embed assets/tray.ico
var trayIcon []byte

type windowsController struct {
	mu              sync.RWMutex
	started         bool
	available       bool
	proxyRunning    bool
	callbacks       MenuCallbacks
	stopLoop        func()
	openItem        *systray.MenuItem
	toggleProxyItem *systray.MenuItem
	exitItem        *systray.MenuItem
	readyOnce       sync.Once
	exitOnce        sync.Once
	readyCh         chan struct{}
	exitCh          chan struct{}
}

func newPlatformController() Controller {
	return &windowsController{
		readyCh: make(chan struct{}),
		exitCh:  make(chan struct{}),
	}
}

func (c *windowsController) Supported() bool {
	return true
}

func (c *windowsController) Available() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.available
}

func (c *windowsController) Start(_ context.Context, callbacks MenuCallbacks) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return nil
	}
	c.started = true
	c.callbacks = callbacks
	if callbacks.IsProxyRunning != nil {
		c.proxyRunning = callbacks.IsProxyRunning()
	}
	c.mu.Unlock()

	startLoop, stopLoop := systray.RunWithExternalLoop(c.onReady, c.onExit)
	c.mu.Lock()
	c.stopLoop = stopLoop
	c.mu.Unlock()
	startLoop()
	return nil
}

func (c *windowsController) SetProxyRunning(running bool) {
	c.mu.Lock()
	c.proxyRunning = running
	item := c.toggleProxyItem
	c.mu.Unlock()

	if item != nil {
		item.SetTitle(proxyToggleTitle(running))
	}
}

func (c *windowsController) Close() error {
	c.mu.Lock()
	stopLoop := c.stopLoop
	c.stopLoop = nil
	c.mu.Unlock()
	if stopLoop != nil {
		stopLoop()
		return nil
	}
	systray.Quit()
	return nil
}

func (c *windowsController) onReady() {
	systray.SetTitle("CLIRO")
	systray.SetTooltip("CLIRO")
	if len(trayIcon) > 0 {
		systray.SetIcon(trayIcon)
	}

	c.mu.Lock()
	proxyRunning := c.proxyRunning
	c.openItem = systray.AddMenuItem("Open / Bring to Front", "Restore app window")
	c.toggleProxyItem = systray.AddMenuItem(proxyToggleTitle(proxyRunning), "Toggle API Router Proxy")
	systray.AddSeparator()
	c.exitItem = systray.AddMenuItem("Exit App", "Exit CLIRO")
	c.available = true

	// Capture channels while holding lock to prevent race
	openCh := c.openItem.ClickedCh
	toggleCh := c.toggleProxyItem.ClickedCh
	exitCh := c.exitItem.ClickedCh
	c.mu.Unlock()

	c.readyOnce.Do(func() { close(c.readyCh) })

	c.mu.RLock()
	onReady := c.callbacks.OnReady
	c.mu.RUnlock()
	if onReady != nil {
		onReady()
	}

	go c.listenMenu(openCh, toggleCh, exitCh)
}

func (c *windowsController) onExit() {
	c.mu.Lock()
	c.available = false
	c.mu.Unlock()
	c.exitOnce.Do(func() { close(c.exitCh) })
}

func (c *windowsController) listenMenu(openCh, toggleCh, exitCh <-chan struct{}) {
	for {
		select {
		case <-openCh:
			c.mu.RLock()
			onOpen := c.callbacks.OnOpen
			c.mu.RUnlock()
			if onOpen != nil {
				onOpen()
			}
		case <-toggleCh:
			c.mu.RLock()
			onToggle := c.callbacks.OnToggleProxy
			isRunning := c.callbacks.IsProxyRunning
			c.mu.RUnlock()
			if onToggle != nil {
				if err := onToggle(); err == nil && isRunning != nil {
					c.SetProxyRunning(isRunning())
				}
			}
		case <-exitCh:
			c.mu.RLock()
			onExit := c.callbacks.OnExit
			c.mu.RUnlock()
			if onExit != nil {
				onExit()
			}
		case <-c.exitCh:
			return
		}
	}
}

func proxyToggleTitle(running bool) string {
	if running {
		return "Disable API Router Proxy"
	}
	return "Enable API Router Proxy"
}
