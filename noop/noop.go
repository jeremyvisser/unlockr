package noop

import (
	"context"
	"fmt"
	"log"

	"jeremy.visser.name/go/unlockr/device"
)

type Device struct {
	device.Base

	Chaotic int `json:"chaotic"`
	chaos   int
}

func (d *Device) isChaos() error {
	if d.Chaotic > 0 {
		d.chaos++
		if luck := d.chaos % d.Chaotic; luck == 0 {
			return fmt.Errorf("chaos monkey struck again! (unluckiness=1/%d)", d.Chaotic)
		}
	}
	return nil
}

func (d *Device) Power(ctx context.Context, on bool) error {
	log.Printf("Noop[%s] powered on=%v", d.GetName(), on)
	if err := d.isChaos(); err != nil {
		log.Printf("Noop[%s] had an error, incredibly: %v", d.GetName(), err)
		return err
	}
	return nil
}
