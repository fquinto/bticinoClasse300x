package homekit

import (
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/service"
	"github.com/sirupsen/logrus"
)

// LockAccessory represents a HomeKit lock
type LockAccessory struct {
	*accessory.A
	Lock   *service.LockMechanism
	logger *logrus.Logger
}

// NewLockAccessory creates a new lock accessory
func NewLockAccessory(logger *logrus.Logger) (*LockAccessory, error) {
	info := accessory.Info{
		Name:         "BTicino Door Lock",
		Manufacturer: "BTicino",
		Model:        "Class 300X Lock",
		SerialNumber: "LOCK-001",
	}

	acc := accessory.New(info, accessory.TypeDoorLock)

	// Add lock mechanism service
	lockService := service.NewLockMechanism()
	acc.AddS(lockService.S)

	lock := &LockAccessory{
		A:      acc,
		Lock:   lockService,
		logger: logger,
	}

	return lock, nil
}

// SetLocked sets the lock state
func (l *LockAccessory) SetLocked(locked bool) {
	l.logger.WithField("locked", locked).Info("Door lock state changed")
	if locked {
		l.Lock.LockCurrentState.SetValue(1) // Secured
		l.Lock.LockTargetState.SetValue(1)  // Secured
	} else {
		l.Lock.LockCurrentState.SetValue(0) // Unsecured
		l.Lock.LockTargetState.SetValue(0)  // Unsecured
	}
}

// IsLocked returns the current lock state
func (l *LockAccessory) IsLocked() bool {
	return l.Lock.LockCurrentState.Value() == 1
}
