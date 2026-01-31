//go:build windows

package hotkey

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Register installs hotkeys and wires them to handler.
func Register(startKey, pauseKey, cancelKey string, hook bool, handler func(id int), debug bool) error {
	if hook {
		return startLowLevelHook(startKey, pauseKey, cancelKey, handler, debug)
	}
	return registerHotkeys(startKey, pauseKey, cancelKey, handler, debug)
}

func registerHotkeys(startKey, pauseKey, cancelKey string, handler func(id int), debug bool) error {
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

	errCh := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		for i := range defs {
			mod, vk, err := parseHotkey(defs[i].spec)
			if err != nil {
				errCh <- fmt.Errorf("invalid hotkey '%s': %v", defs[i].spec, err)
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

		for _, d := range defs {
			r, _, _ := procRegisterHotKey.Call(
				0,
				uintptr(d.id),
				uintptr(d.mod),
				uintptr(d.vk),
			)
			if r == 0 {
				for _, od := range defs {
					if od.id == d.id {
						break
					}
					procUnregisterHotKey.Call(0, uintptr(od.id))
				}
				errCh <- fmt.Errorf("RegisterHotKey failed for '%s' (id=%d)", d.spec, d.id)
				return
			}
			if debug {
				fmt.Printf("[hotkey-debug] RegisterHotKey succeeded for id=%d spec=%s\n", d.id, d.spec)
			}
		}

		if debug {
			fmt.Printf("[hotkey] Registered global hotkeys: start=%s pause=%s cancel=%s\n", startKey, pauseKey, cancelKey)
		}
		errCh <- nil

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
	case err := <-errCh:
		return err
	case <-time.After(2 * time.Second):
		return fmt.Errorf("timeout registering hotkeys")
	}
}

func startLowLevelHook(startKey, pauseKey, cancelKey string, handler func(id int), debug bool) error {
	type candidate struct {
		id  int
		mod uint32
	}

	errCh := make(chan error, 1)
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
				errCh <- fmt.Errorf("invalid hotkey '%s': %v", s.spec, err)
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
		procGetAsyncKeyState := user32.NewProc("GetAsyncKeyState")

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
							go handler(c.id)
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
			errCh <- fmt.Errorf("SetWindowsHookExW failed")
			return
		}

		if debug {
			fmt.Printf("[hotkey] low-level hook installed (WH_KEYBOARD_LL)\n")
		}

		errCh <- nil

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

		procUnhookWindowsHookEx.Call(hook)
		if debug {
			fmt.Println("[hotkey] low-level hook uninstalled")
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-time.After(2 * time.Second):
		return fmt.Errorf("timeout installing low-level hook")
	}
}

const (
	VK_NUMPAD0  = 0x60
	VK_NUMPAD1  = 0x61
	VK_NUMPAD2  = 0x62
	VK_NUMPAD3  = 0x63
	VK_NUMPAD4  = 0x64
	VK_NUMPAD5  = 0x65
	VK_NUMPAD6  = 0x66
	VK_NUMPAD7  = 0x67
	VK_NUMPAD8  = 0x68
	VK_NUMPAD9  = 0x69
	VK_ADD      = 0x6B
	VK_SUBTRACT = 0x6D
)

// parseHotkey accepts strings like "alt+q", "ctrl+shift+F1", "esc" and returns modifier mask and vk.
func parseHotkey(s string) (uint32, uint32, error) {
	if s == "" {
		return 0, 0, fmt.Errorf("empty key")
	}
	parts := strings.Split(s, "+")
	for i := range parts {
		parts[i] = strings.TrimSpace(strings.ToLower(parts[i]))
	}
	var mod uint32
	var keyToken string
	if len(parts) == 1 {
		keyToken = parts[0]
	} else {
		keyToken = parts[len(parts)-1]
		for _, p := range parts[:len(parts)-1] {
			switch p {
			case "alt", "menu":
				mod |= 0x0001
			case "ctrl", "control":
				mod |= 0x0002
			case "shift":
				mod |= 0x0004
			case "win", "meta", "super":
				mod |= 0x0008
			default:
			}
		}
	}
	if len(keyToken) == 1 {
		ch := keyToken[0]
		if ch >= 'a' && ch <= 'z' {
			return mod, uint32(ch - 'a' + 'A'), nil
		}
		if ch >= '0' && ch <= '9' {
			return mod, uint32(ch), nil
		}
	}
	switch keyToken {
	case "esc", "escape":
		return mod, 0x1B, nil
	case "space":
		return mod, 0x20, nil
	case "enter", "return":
		return mod, 0x0D, nil
	}
	if strings.HasPrefix(keyToken, "f") {
		nStr := strings.TrimPrefix(keyToken, "f")
		if n, err := strconv.Atoi(nStr); err == nil && n >= 1 && n <= 24 {
			return mod, 0x70 + uint32(n-1), nil
		}
	}
	switch keyToken {
	case "numpad0", "num0", "kp0":
		return mod, VK_NUMPAD0, nil
	case "numpad1", "num1", "kp1":
		return mod, VK_NUMPAD1, nil
	case "numpad2", "num2", "kp2":
		return mod, VK_NUMPAD2, nil
	case "numpad3", "num3", "kp3":
		return mod, VK_NUMPAD3, nil
	case "numpad4", "num4", "kp4":
		return mod, VK_NUMPAD4, nil
	case "numpad5", "num5", "kp5":
		return mod, VK_NUMPAD5, nil
	case "numpad6", "num6", "kp6":
		return mod, VK_NUMPAD6, nil
	case "numpad7", "num7", "kp7":
		return mod, VK_NUMPAD7, nil
	case "numpad8", "num8", "kp8":
		return mod, VK_NUMPAD8, nil
	case "numpad9", "num9", "kp9":
		return mod, VK_NUMPAD9, nil
	case "add", "plus", "kpadd":
		return mod, VK_ADD, nil
	case "subtract", "minus", "kpsubtract":
		return mod, VK_SUBTRACT, nil
	}

	named := map[string]uint32{
		"tab":       0x09,
		"backspace": 0x08,
		"insert":    0x2D,
		"delete":    0x2E,
		"home":      0x24,
		"end":       0x23,
		"pageup":    0x21,
		"pagedown":  0x22,
		"left":      0x25,
		"up":        0x26,
		"right":     0x27,
		"down":      0x28,
	}
	if v, ok := named[keyToken]; ok {
		return mod, v, nil
	}
	if len(keyToken) == 1 {
		return mod, uint32(strings.ToUpper(keyToken)[0]), nil
	}
	return 0, 0, fmt.Errorf("unsupported key token: %s", s)
}
