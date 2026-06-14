package homekit

import (
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/service"
	"github.com/sirupsen/logrus"
)

// DoorbellAccessory represents a HomeKit doorbell using a switch
type DoorbellAccessory struct {
	*accessory.A
	Switch *service.Switch
	logger *logrus.Logger
}

// NewDoorbellAccessory creates a new doorbell accessory
func NewDoorbellAccessory(logger *logrus.Logger) (*DoorbellAccessory, error) {
	info := accessory.Info{
		Name:         "BTicino Doorbell",
		Manufacturer: "BTicino",
		Model:        "Class 300X Doorbell",
		SerialNumber: "DOORBELL-001",
	}

	acc := accessory.New(info, accessory.TypeOther)

	// Add switch service for doorbell
	switchService := service.NewSwitch()
	acc.AddS(switchService.S)

	doorbell := &DoorbellAccessory{
		A:      acc,
		Switch: switchService,
		logger: logger,
	}

	// Set up switch callback - simplified version without callback
	// The HAP library will handle the switch updates

	return doorbell, nil
}

// Ring triggers the doorbell ring event
func (d *DoorbellAccessory) Ring() {
	d.logger.Info("Doorbell ring detected - updating HomeKit")
	d.Switch.On.SetValue(true)
}
