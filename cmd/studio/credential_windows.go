//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

type dataBlob struct {
	size uint32
	data *byte
}

var crypt32 = syscall.NewLazyDLL("crypt32.dll")
var protectProc = crypt32.NewProc("CryptProtectData")
var unprotectProc = crypt32.NewProc("CryptUnprotectData")
var localFree = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")

func dpapi(raw []byte, encrypt bool) ([]byte, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty credential data")
	}
	input := dataBlob{size: uint32(len(raw)), data: &raw[0]}
	var output dataBlob
	proc := protectProc
	if !encrypt {
		proc = unprotectProc
	}
	r, _, e := proc.Call(uintptr(unsafe.Pointer(&input)), 0, 0, 0, 0, 0x1, uintptr(unsafe.Pointer(&output)))
	if r == 0 {
		return nil, fmt.Errorf("Windows credential encryption failed: %w", e)
	}
	defer localFree.Call(uintptr(unsafe.Pointer(output.data)))
	return append([]byte(nil), unsafe.Slice(output.data, int(output.size))...), nil
}
func protectAccount(b []byte) ([]byte, error)   { return dpapi(b, true) }
func unprotectAccount(b []byte) ([]byte, error) { return dpapi(b, false) }
