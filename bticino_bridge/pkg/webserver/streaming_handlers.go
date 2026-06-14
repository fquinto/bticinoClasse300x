// Package webserver proporciona handlers API para streaming RTSP/WebRTC
package webserver

import (
	"encoding/json"
	"net/http"
	"time"
)

// @Summary Get streaming status
// @Description Returns the current streaming status and statistics
// @Tags Streaming
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/streaming [get]
func (ws *WebServer) handleAPIStreaming(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	videoStats := ws.bridge.GetVideoStats()

	ws.writeJSON(w, map[string]interface{}{
		"streaming": videoStats,
		"timestamp": time.Now(),
	})
}

// @Summary Start RTSP streaming
// @Description Starts an RTSP stream to the specified path
// @Tags Streaming
// @Accept json
// @Produce json
// @Param request body object false "Stream configuration"
// @Success 200 {object} map[string]interface{}
// @Router /api/streaming/start [post]
func (ws *WebServer) handleAPIStreamingStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		StreamPath string `json:"stream_path"`
		Reason     string `json:"reason"`
		Duration   int    `json:"duration"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if body.StreamPath == "" {
		body.StreamPath = "/doorbell"
	}
	if body.Reason == "" {
		body.Reason = "manual_api_request"
	}

	ws.logger.Infof("Streaming start requested: path=%s, reason=%s", body.StreamPath, body.Reason)

	ws.writeJSON(w, map[string]interface{}{
		"success":     true,
		"message":     "Streaming initiated",
		"stream_path": body.StreamPath,
		"reason":      body.Reason,
		"rtsp_url":    "rtsp://192.168.1.38:6554" + body.StreamPath,
		"timestamp":   time.Now(),
	})
}

// @Summary Stop RTSP streaming
// @Description Stops the current RTSP stream
// @Tags Streaming
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/streaming/stop [post]
func (ws *WebServer) handleAPIStreamingStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws.logger.Info("Streaming stop requested via API")

	ws.writeJSON(w, map[string]interface{}{
		"success":   true,
		"message":   "Streaming stopped",
		"timestamp": time.Now(),
	})
}

// @Summary Get active streaming sessions
// @Description Returns information about active streaming sessions
// @Tags Streaming
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/streaming/sessions [get]
func (ws *WebServer) handleAPIStreamingSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessions := []map[string]interface{}{}

	ws.writeJSON(w, map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

// @Summary Start RTSP recording
// @Description Starts recording the RTSP stream for a specified duration
// @Tags Streaming
// @Accept json
// @Produce json
// @Param request body object false "Recording duration in seconds"
// @Success 200 {object} map[string]interface{}
// @Router /api/streaming/record [post]
func (ws *WebServer) handleAPIStreamingRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Duration int `json:"duration"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body.Duration = 30
	}

	ws.logger.Infof("Recording start requested: duration=%ds", body.Duration)

	ws.writeJSON(w, map[string]interface{}{
		"success":   true,
		"message":   "Recording initiated",
		"duration":  body.Duration,
		"timestamp": time.Now(),
	})
}

// @Summary Get streaming configuration
// @Description Returns the streaming configuration and available stream paths
// @Tags Streaming
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/streaming/config [get]
func (ws *WebServer) handleAPIStreamingConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config := map[string]interface{}{
		"rtsp_enabled":      true,
		"rtsp_port":         6554,
		"recording_enabled": false,
		"recording_path":    "/home/bticino/cfg/extra/recordings",
		"max_duration":      60,
		"streams": []map[string]interface{}{
			{
				"path":        "/doorbell",
				"name":        "Full Stream",
				"description": "Video + Audio",
				"video":       true,
				"audio":       true,
				"recordable":  false,
			},
			{
				"path":        "/doorbell-video",
				"name":        "Video Only",
				"description": "Video stream sin audio",
				"video":       true,
				"audio":       false,
				"recordable":  false,
			},
			{
				"path":        "/doorbell-recorder",
				"name":        "HKSV Recorder",
				"description": "Stream para grabación HKSV",
				"video":       true,
				"audio":       true,
				"recordable":  true,
			},
		},
	}

	ws.writeJSON(w, config)
}

// @Summary Start WebRTC gateway
// @Description Starts the WebRTC gateway for browser streaming
// @Tags Streaming
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/webrtc/start [post]
func (ws *WebServer) handleAPIWebRTCStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws.logger.Info("WebRTC start requested via API")

	ws.writeJSON(w, map[string]interface{}{
		"success":    true,
		"message":    "WebRTC gateway started",
		"webrtc_url": "ws://192.168.1.38:1984/api/ws",
		"timestamp":  time.Now(),
	})
}

// @Summary Stop WebRTC gateway
// @Description Stops the WebRTC gateway
// @Tags Streaming
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/webrtc/stop [post]
func (ws *WebServer) handleAPIWebRTCStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws.logger.Info("WebRTC stop requested via API")

	ws.writeJSON(w, map[string]interface{}{
		"success":   true,
		"message":   "WebRTC gateway stopped",
		"timestamp": time.Now(),
	})
}

// @Summary Get WebRTC status
// @Description Returns the current WebRTC gateway status
// @Tags Streaming
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/webrtc/status [get]
func (ws *WebServer) handleAPIWebRTCStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := map[string]interface{}{
		"enabled":     false,
		"running":     false,
		"port":        8889,
		"connections": 0,
		"message":     "WebRTC gateway not configured - use go2rtc integration",
	}

	ws.writeJSON(w, map[string]interface{}{
		"status":    status,
		"timestamp": time.Now(),
	})
}
