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

package main

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"
)

const (
	gwOwner = 4

	clsctxInprocServer      = 0x1
	coinitApartmentThreaded = 0x2
	rpcEChangedMode         = 0x80010106
)

var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	ole32                      = syscall.NewLazyDLL("ole32.dll")
	procCoCreateInstance       = ole32.NewProc("CoCreateInstance")
	procCoInitializeEx         = ole32.NewProc("CoInitializeEx")
	procCoUninitialize         = ole32.NewProc("CoUninitialize")
	procEnumWindows            = user32.NewProc("EnumWindows")
	procGetWindow              = user32.NewProc("GetWindow")
	procGetWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible        = user32.NewProc("IsWindowVisible")
	clsidTaskbarList           = guid{0x56FDF344, 0xFD6D, 0x11D0, [8]byte{0x95, 0x8A, 0x00, 0x60, 0x97, 0xC9, 0xA0, 0x90}}
	iidTaskbarList             = guid{0x56FDF342, 0xFD6D, 0x11D0, [8]byte{0x95, 0x8A, 0x00, 0x60, 0x97, 0xC9, 0xA0, 0x90}}
)

func applyTaskbarVisibility(visible bool) {
	hwnd := findAppWindow()
	if hwnd == 0 {
		log.Printf("taskbar visibility: app window not found")
		return
	}

	if err := setTaskbarTabVisible(hwnd, visible); err != nil {
		log.Printf("taskbar visibility failed: %v", err)
	}
}

type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

type taskbarList struct {
	vtbl *taskbarListVtbl
}

type taskbarListVtbl struct {
	queryInterface uintptr
	addRef         uintptr
	release        uintptr
	hrInit         uintptr
	addTab         uintptr
	deleteTab      uintptr
	activateTab    uintptr
	setActiveTab   uintptr
}

func setTaskbarTabVisible(hwnd uintptr, visible bool) error {
	taskbar, uninitialize, err := createTaskbarList()
	if uninitialize {
		defer procCoUninitialize.Call()
	}
	if err != nil {
		return err
	}
	defer syscall.SyscallN(taskbar.vtbl.release, uintptr(unsafe.Pointer(taskbar)))

	if hr := callCOM(taskbar.vtbl.hrInit, uintptr(unsafe.Pointer(taskbar))); failed(hr) {
		return hresultError("ITaskbarList.HrInit", hr)
	}

	if visible {
		if hr := callCOM(taskbar.vtbl.addTab, uintptr(unsafe.Pointer(taskbar)), hwnd); failed(hr) {
			return hresultError("ITaskbarList.AddTab", hr)
		}
		return nil
	}

	if hr := callCOM(taskbar.vtbl.deleteTab, uintptr(unsafe.Pointer(taskbar)), hwnd); failed(hr) {
		return hresultError("ITaskbarList.DeleteTab", hr)
	}
	return nil
}

func createTaskbarList() (*taskbarList, bool, error) {
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	uninitialize := hr == 0 || hr == 1
	if failed(hr) && uint32(hr) != rpcEChangedMode {
		return nil, false, hresultError("CoInitializeEx", hr)
	}

	var instance *taskbarList
	hr, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidTaskbarList)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidTaskbarList)),
		uintptr(unsafe.Pointer(&instance)),
	)
	if failed(hr) {
		if uninitialize {
			procCoUninitialize.Call()
		}
		return nil, false, hresultError("CoCreateInstance(CLSID_TaskbarList)", hr)
	}
	return instance, uninitialize, nil
}

func callCOM(fn uintptr, args ...uintptr) uintptr {
	result, _, _ := syscall.SyscallN(fn, args...)
	return result
}

func failed(hr uintptr) bool {
	return int32(uint32(hr)) < 0
}

func hresultError(name string, hr uintptr) error {
	return fmt.Errorf("%s failed with HRESULT 0x%08X", name, uint32(hr))
}

func findAppWindow() uintptr {
	currentPID := uint32(os.Getpid())
	var found uintptr

	callback := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		if found != 0 {
			return 0
		}
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		if visible == 0 {
			return 1
		}
		owner, _, _ := procGetWindow.Call(hwnd, gwOwner)
		if owner != 0 {
			return 1
		}

		var windowPID uint32
		procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
		if windowPID != currentPID {
			return 1
		}

		found = hwnd
		return 0
	})

	procEnumWindows.Call(callback, 0)
	return found
}
