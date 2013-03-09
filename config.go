package main

import (
	"os"
)

type PinkyConfig struct {
	LxcDir string
}

func InitPinkyConfig() *PinkyConfig {
	p := &PinkyConfig{}
	p.LxcDir = os.Getenv("LXC_DIR")
	if p.LxcDir == "" {
		p.LxcDir = "/home/vagrant/lxc"
	}
	return p
}
