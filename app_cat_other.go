//go:build !windows

package main

func supportsBackdrop() bool { return false }

const colorKeyR, colorKeyG, colorKeyB = 0, 0, 0

func cutOutBackground(title string) {}
