//go:build !windows

package main

// On non-Windows systems credential storage inherits 0600 file permissions.
func protectAccount(b []byte) ([]byte, error)   { return b, nil }
func unprotectAccount(b []byte) ([]byte, error) { return b, nil }
