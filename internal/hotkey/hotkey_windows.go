// Copyright (C) 2026 Joey Kot <joey.kot.x@gmail.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed WITHOUT ANY WARRANTY; without even the
// implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See <https://www.gnu.org/licenses/> for more details.

//go:build windows

package hotkey

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Registration represents a registered hotkey set.
type Registration struct {
	once sync.Once
	stop func()
}

// Stop releases registered hotkeys.
func (r *Registration) Stop() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		if r.stop != nil {
			r.stop()
		}
	})
}

// Register installs hotkeys and wires them to handler.
// The handler runs synchronously on the hotkey thread and must return promptly.
func Register(startKey, pauseKey, cancelKey string, hook bool, handler func(id int), debug bool) error {
	_, err := RegisterWithStop(startKey, pauseKey, cancelKey, hook, handler, debug)
	return err
}

// RegisterWithStop installs hotkeys and returns a handle that can unregister them.
// The handler runs synchronously on the hotkey thread and must return promptly.
func RegisterWithStop(startKey, pauseKey, cancelKey string, hook bool, handler func(id int), debug bool) (*Registration, error) {
	if err := ValidateBindings(startKey, pauseKey, cancelKey); err != nil {
		return nil, err
	}
	if hook {
		return startLowLevelHook(startKey, pauseKey, cancelKey, handler, debug)
	}
	return registerHotkeys(startKey, pauseKey, cancelKey, handler, debug)
}

func registerHotkeys(startKey, pauseKey, cancelKey string, handler func(id int), debug bool) (*Registration, error) {
	type hotkeyDef struct {
		id   int
		spec string
		mod  uint32
		vk   uint32
	}
	defs := []hotkeyDef{
		{id: 1, spec: startKey},
		{id: 2, spec: pauseKey},
		{id: 3, spec: cancelKey},
	}

	type result struct {
		reg *Registration
		err error
	}
	resultCh := make(chan result, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		for i := range defs {
			mod, vk, err := parseHotkey(defs[i].spec)
			if err != nil {
				resultCh <- result{err: fmt.Errorf("invalid hotkey '%s': %v", defs[i].spec, err)}
				return
			}
			defs[i].mod = mod
			defs[i].vk = vk
			if debug {
				fmt.Printf("[hotkey-debug] parsed '%s' -> mod=0x%X vk=0x%X\n", defs[i].spec, defs[i].mod, defs[i].vk)
			}
		}

		user32 := syscall.NewLazyDLL("user32.dll")
		procRegisterHotKey := user32.NewProc("RegisterHotKey")
		procUnregisterHotKey := user32.NewProc("UnregisterHotKey")
		procGetMessageW := user32.NewProc("GetMessageW")
		procPostThreadMessageW := user32.NewProc("PostThreadMessageW")
		procGetCurrentThreadId := syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")

		registered := make([]hotkeyDef, 0, len(defs))
		defer func() {
			for _, d := range registered {
				procUnregisterHotKey.Call(0, uintptr(d.id))
			}
		}()
		for _, d := range defs {
			r, _, _ := procRegisterHotKey.Call(
				0,
				uintptr(d.id),
				uintptr(d.mod),
				uintptr(d.vk),
			)
			if r == 0 {
				resultCh <- result{err: fmt.Errorf("RegisterHotKey failed for '%s' (id=%d)", d.spec, d.id)}
				return
			}
			registered = append(registered, d)
			if debug {
				fmt.Printf("[hotkey-debug] RegisterHotKey succeeded for id=%d spec=%s\n", d.id, d.spec)
			}
		}

		if debug {
			fmt.Printf("[hotkey] Registered global hotkeys: start=%s pause=%s cancel=%s\n", startKey, pauseKey, cancelKey)
		}
		threadID, _, _ := procGetCurrentThreadId.Call()
		resultCh <- result{reg: &Registration{stop: func() {
			const WM_QUIT = 0x0012
			procPostThreadMessageW.Call(threadID, uintptr(WM_QUIT), 0, 0)
		}}}

		var msg struct {
			Hwnd    uintptr
			Message uint32
			WParam  uintptr
			LParam  uintptr
			Time    uint32
			Pt_x    int32
			Pt_y    int32
		}
		const WM_HOTKEY = 0x0312
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(ret) == -1 {
				fmt.Println("[hotkey] GetMessageW error; exiting hotkey loop")
				return
			}
			if ret == 0 {
				if debug {
					fmt.Println("[hotkey] hotkey loop stopped")
				}
				return
			}
			if debug {
				fmt.Printf("[hotkey-debug] msg: Message=0x%X WParam=0x%X LParam=0x%X\n", msg.Message, msg.WParam, msg.LParam)
			}
			if msg.Message == WM_HOTKEY {
				id := int(msg.WParam)
				if debug {
					fmt.Printf("[hotkey-debug] WM_HOTKEY received id=%d\n", id)
				}
				handler(id)
			}
		}
	}()

	select {
	case res := <-resultCh:
		return res.reg, res.err
	case <-time.After(2 * time.Second):
		return nil, fmt.Errorf("timeout registering hotkeys")
	}
}

func startLowLevelHook(startKey, pauseKey, cancelKey string, handler func(id int), debug bool) (*Registration, error) {
	type candidate struct {
		id  int
		mod uint32
	}

	type result struct {
		reg *Registration
		err error
	}
	resultCh := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		specs := []struct {
			id   int
			spec string
		}{
			{id: 1, spec: startKey},
			{id: 2, spec: pauseKey},
			{id: 3, spec: cancelKey},
		}

		lookup := make(map[uint32][]candidate)
		for _, s := range specs {
			mod, vk, err := parseHotkey(s.spec)
			if err != nil {
				resultCh <- result{err: fmt.Errorf("invalid hotkey '%s': %v", s.spec, err)}
				return
			}
			lookup[vk] = append(lookup[vk], candidate{id: s.id, mod: mod})
			if debug {
				fmt.Printf("[hotkey-debug] parsed '%s' -> mod=0x%X vk=0x%X\n", s.spec, mod, vk)
			}
		}

		user32 := syscall.NewLazyDLL("user32.dll")
		procSetWindowsHookExW := user32.NewProc("SetWindowsHookExW")
		procUnhookWindowsHookEx := user32.NewProc("UnhookWindowsHookEx")
		procCallNextHookEx := user32.NewProc("CallNextHookEx")
		procGetMessageW := user32.NewProc("GetMessageW")
		procPostThreadMessageW := user32.NewProc("PostThreadMessageW")
		procGetAsyncKeyState := user32.NewProc("GetAsyncKeyState")
		procGetCurrentThreadId := syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")

		const (
			WH_KEYBOARD_LL = 13
			WM_KEYDOWN     = 0x0100
			WM_KEYUP       = 0x0101
			WM_SYSKEYDOWN  = 0x0104
			WM_SYSKEYUP    = 0x0105
			LLKHF_INJECTED = 0x10
			VK_SHIFT       = 0x10
			VK_CONTROL     = 0x11
			VK_MENU        = 0x12
			VK_LWIN        = 0x5B
			VK_RWIN        = 0x5C
		)

		type KBDLLHOOKSTRUCT struct {
			vkCode      uint32
			scanCode    uint32
			flags       uint32
			time        uint32
			dwExtraInfo uintptr
		}

		modsSatisfied := func(required uint32) bool {
			if required == 0 {
				return true
			}
			if (required & 0x0002) != 0 {
				st, _, _ := procGetAsyncKeyState.Call(uintptr(VK_CONTROL))
				if (st & 0x8000) == 0 {
					return false
				}
			}
			if (required & 0x0001) != 0 {
				st, _, _ := procGetAsyncKeyState.Call(uintptr(VK_MENU))
				if (st & 0x8000) == 0 {
					return false
				}
			}
			if (required & 0x0004) != 0 {
				st, _, _ := procGetAsyncKeyState.Call(uintptr(VK_SHIFT))
				if (st & 0x8000) == 0 {
					return false
				}
			}
			if (required & 0x0008) != 0 {
				stL, _, _ := procGetAsyncKeyState.Call(uintptr(VK_LWIN))
				stR, _, _ := procGetAsyncKeyState.Call(uintptr(VK_RWIN))
				if (stL&0x8000) == 0 && (stR&0x8000) == 0 {
					return false
				}
			}
			return true
		}

		swallowed := make(map[uint32]bool)

		callback := syscall.NewCallback(func(nCode, wParam, lParam uintptr) uintptr {
			if int32(nCode) < 0 {
				ret, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
				return ret
			}

			msg := uint32(wParam)
			k := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
			vk := k.vkCode
			flags := k.flags

			if (flags & LLKHF_INJECTED) != 0 {
				ret, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
				return ret
			}

			if msg == WM_KEYDOWN || msg == WM_SYSKEYDOWN {
				if cands, ok := lookup[vk]; ok {
					for _, c := range cands {
						if modsSatisfied(c.mod) {
							swallowed[vk] = true
							if debug {
								fmt.Printf("[hotkey-debug] swallowed keydown vk=0x%X id=%d\n", vk, c.id)
							}
							handler(c.id)
							return uintptr(1)
						}
					}
				}
			}

			if msg == WM_KEYUP || msg == WM_SYSKEYUP {
				if swallowed[vk] {
					if debug {
						fmt.Printf("[hotkey-debug] swallowed keyup vk=0x%X\n", vk)
					}
					delete(swallowed, vk)
					return uintptr(1)
				}
			}

			ret, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
			return ret
		})

		hook, _, _ := procSetWindowsHookExW.Call(
			uintptr(WH_KEYBOARD_LL),
			callback,
			0,
			0,
		)
		if hook == 0 {
			resultCh <- result{err: fmt.Errorf("SetWindowsHookExW failed")}
			return
		}
		defer procUnhookWindowsHookEx.Call(hook)

		if debug {
			fmt.Printf("[hotkey] low-level hook installed (WH_KEYBOARD_LL)\n")
		}

		threadID, _, _ := procGetCurrentThreadId.Call()
		resultCh <- result{reg: &Registration{stop: func() {
			const WM_QUIT = 0x0012
			procPostThreadMessageW.Call(threadID, uintptr(WM_QUIT), 0, 0)
		}}}

		var msg struct {
			Hwnd    uintptr
			Message uint32
			WParam  uintptr
			LParam  uintptr
			Time    uint32
			Pt_x    int32
			Pt_y    int32
		}
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(ret) == -1 {
				if debug {
					fmt.Println("[hotkey] GetMessageW error; exiting low-level hook loop")
				}
				break
			}
			if ret == 0 {
				break
			}
		}

		if debug {
			fmt.Println("[hotkey] low-level hook uninstalled")
		}
	}()

	select {
	case res := <-resultCh:
		return res.reg, res.err
	case <-time.After(2 * time.Second):
		return nil, fmt.Errorf("timeout installing low-level hook")
	}
}
