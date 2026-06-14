package input

import (
	"fmt"
	"os"
	"path/filepath"
)

// GPIOManager handles GPIO pin management and export/unexport operations
type GPIOManager struct {
	exportedPins map[int]bool
}

// NewGPIOManager creates a new GPIO manager
func NewGPIOManager() *GPIOManager {
	return &GPIOManager{
		exportedPins: make(map[int]bool),
	}
}

// ExportGPIO exports a GPIO pin for userspace access
func (gm *GPIOManager) ExportGPIO(pin int) error {
	if gm.exportedPins[pin] {
		return nil // Already exported
	}

	exportPath := "/sys/class/gpio/export"
	pinStr := fmt.Sprintf("%d", pin)

	// Check if GPIO is already exported
	gpioPath := fmt.Sprintf("/sys/class/gpio/gpio%d", pin)
	if _, err := os.Stat(gpioPath); err == nil {
		gm.exportedPins[pin] = true
		return nil // Already exported by system
	}

	// Export the GPIO
	file, err := os.OpenFile(exportPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open GPIO export: %v", err)
	}
	defer file.Close()

	_, err = file.WriteString(pinStr)
	if err != nil {
		return fmt.Errorf("failed to export GPIO %d: %v", pin, err)
	}

	gm.exportedPins[pin] = true
	return nil
}

// UnexportGPIO unexports a GPIO pin
func (gm *GPIOManager) UnexportGPIO(pin int) error {
	if !gm.exportedPins[pin] {
		return nil // Not exported by us
	}

	unexportPath := "/sys/class/gpio/unexport"
	pinStr := fmt.Sprintf("%d", pin)

	file, err := os.OpenFile(unexportPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open GPIO unexport: %v", err)
	}
	defer file.Close()

	_, err = file.WriteString(pinStr)
	if err != nil {
		return fmt.Errorf("failed to unexport GPIO %d: %v", pin, err)
	}

	delete(gm.exportedPins, pin)
	return nil
}

// SetGPIODirection sets the direction of a GPIO pin (in/out)
func (gm *GPIOManager) SetGPIODirection(pin int, direction string) error {
	dirPath := fmt.Sprintf("/sys/class/gpio/gpio%d/direction", pin)

	file, err := os.OpenFile(dirPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open GPIO %d direction file: %v", pin, err)
	}
	defer file.Close()

	_, err = file.WriteString(direction)
	if err != nil {
		return fmt.Errorf("failed to set GPIO %d direction to %s: %v", pin, direction, err)
	}

	return nil
}

// SetGPIOValue sets the value of an output GPIO pin (0/1)
func (gm *GPIOManager) SetGPIOValue(pin int, value bool) error {
	valuePath := fmt.Sprintf("/sys/class/gpio/gpio%d/value", pin)

	file, err := os.OpenFile(valuePath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open GPIO %d value file: %v", pin, err)
	}
	defer file.Close()

	valueStr := "0"
	if value {
		valueStr = "1"
	}

	_, err = file.WriteString(valueStr)
	if err != nil {
		return fmt.Errorf("failed to set GPIO %d value to %s: %v", pin, valueStr, err)
	}

	return nil
}

// GetGPIOValue reads the current value of a GPIO pin
func (gm *GPIOManager) GetGPIOValue(pin int) (bool, error) {
	valuePath := fmt.Sprintf("/sys/class/gpio/gpio%d/value", pin)

	data, err := os.ReadFile(valuePath)
	if err != nil {
		return false, fmt.Errorf("failed to read GPIO %d value: %v", pin, err)
	}

	value := string(data)
	value = value[:len(value)-1] // Remove newline

	if value == "1" {
		return true, nil
	} else if value == "0" {
		return false, nil
	}

	return false, fmt.Errorf("unexpected GPIO %d value: %s", pin, value)
}

// IsGPIOExported checks if a GPIO pin is exported
func (gm *GPIOManager) IsGPIOExported(pin int) bool {
	gpioPath := fmt.Sprintf("/sys/class/gpio/gpio%d", pin)
	_, err := os.Stat(gpioPath)
	return err == nil
}

// GetAvailableGPIOs returns list of available GPIO pins on the system
func (gm *GPIOManager) GetAvailableGPIOs() ([]int, error) {
	gpioBasePath := "/sys/class/gpio"

	entries, err := os.ReadDir(gpioBasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read GPIO directory: %v", err)
	}

	var gpios []int
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "." && entry.Name() != ".." {
			name := entry.Name()
			if len(name) > 4 && name[:4] == "gpio" {
				// This is a GPIO directory like gpio12, gpio13, etc.
				var pinNum int
				n, err := fmt.Sscanf(name, "gpio%d", &pinNum)
				if err == nil && n == 1 {
					gpios = append(gpios, pinNum)
				}
			}
		}
	}

	return gpios, nil
}

// GetGPIOInfo returns information about a specific GPIO pin
func (gm *GPIOManager) GetGPIOInfo(pin int) (map[string]string, error) {
	gpioPath := fmt.Sprintf("/sys/class/gpio/gpio%d", pin)

	if !gm.IsGPIOExported(pin) {
		return nil, fmt.Errorf("GPIO %d is not exported", pin)
	}

	info := make(map[string]string)

	// Read direction
	if dirData, err := os.ReadFile(filepath.Join(gpioPath, "direction")); err == nil {
		info["direction"] = string(dirData[:len(dirData)-1]) // Remove newline
	}

	// Read value
	if valueData, err := os.ReadFile(filepath.Join(gpioPath, "value")); err == nil {
		info["value"] = string(valueData[:len(valueData)-1]) // Remove newline
	}

	// Read edge (if available)
	if edgeData, err := os.ReadFile(filepath.Join(gpioPath, "edge")); err == nil {
		info["edge"] = string(edgeData[:len(edgeData)-1]) // Remove newline
	}

	// Read active_low (if available)
	if activeLowData, err := os.ReadFile(filepath.Join(gpioPath, "active_low")); err == nil {
		info["active_low"] = string(activeLowData[:len(activeLowData)-1]) // Remove newline
	}

	return info, nil
}

// CleanupAllGPIOs unexports all GPIOs that were exported by this manager
func (gm *GPIOManager) CleanupAllGPIOs() error {
	var errors []error

	for pin := range gm.exportedPins {
		if err := gm.UnexportGPIO(pin); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("GPIO cleanup errors: %v", errors)
	}

	return nil
}

// MonitorGPIODirectory watches for changes in GPIO directory structure
func MonitorGPIODirectory() ([]string, error) {
	gpioPath := "/sys/class/gpio"

	entries, err := os.ReadDir(gpioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read GPIO directory: %v", err)
	}

	var items []string
	for _, entry := range entries {
		items = append(items, entry.Name())
	}

	return items, nil
}
