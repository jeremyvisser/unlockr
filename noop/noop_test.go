package noop

import (
	"context"
	"testing"

	"jeremy.visser.name/go/unlockr/device"
)

func TestNoopPower(t *testing.T) {
	dl := device.DeviceList{
		"one": &Device{
			Base: device.Base{
				Name: "Noop",
				ACL:  nil,
			},
			Chaotic: 2,
		},
	}

	d, ok := dl["one"]
	if !ok {
		t.Fail()
	}

	p, ok := d.(device.PowerControl)
	if !ok {
		t.Fatal("Device doesn't have PowerControl")
	}

	if err := p.Power(context.Background(), true); err != nil { // expect success (chaos=1/2)
		t.Fatalf("Power: got %v, want nil", err)
	}
	if err := p.Power(context.Background(), true); err == nil { // expect fail (chaos=2/2)
		t.Fatalf("Power: got %v, want !nil", err)
	}
}
