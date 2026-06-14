package homekit

import (
	"github.com/brutella/hap/accessory"
	"github.com/sirupsen/logrus"
)

// CameraAccessory represents a HomeKit camera
type CameraAccessory struct {
	*accessory.Camera
	logger    *logrus.Logger
	streamURL string
	streaming bool
}

// NewCameraAccessory creates a new camera accessory
func NewCameraAccessory(logger *logrus.Logger) (*CameraAccessory, error) {
	info := accessory.Info{
		Name:         "BTicino Camera",
		Manufacturer: "BTicino",
		Model:        "Class 300X Camera",
		SerialNumber: "CAMERA-001",
	}

	camera := accessory.NewCamera(info)

	return &CameraAccessory{
		Camera:    camera,
		logger:    logger,
		streaming: false,
	}, nil
}

// SetStreamURL sets the current stream URL
func (c *CameraAccessory) SetStreamURL(url string) {
	c.streamURL = url
	c.logger.WithField("url", url).Info("Camera stream URL updated")
}

// GetStreamURL returns the current stream URL
func (c *CameraAccessory) GetStreamURL() string {
	return c.streamURL
}

// SetStreamingStatus sets the streaming status
func (c *CameraAccessory) SetStreamingStatus(streaming bool) {
	c.streaming = streaming
	c.logger.WithField("streaming", streaming).Info("Camera streaming status updated")
}

// IsStreaming returns true if the camera is currently streaming
func (c *CameraAccessory) IsStreaming() bool {
	return c.streaming
}
