//go:build !windows && !darwin

package main

func platformGamePorts() (map[int]string, map[int]string, error) {
	return map[int]string{}, map[int]string{}, nil
}
