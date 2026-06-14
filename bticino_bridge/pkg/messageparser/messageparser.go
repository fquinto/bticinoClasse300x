// Package messageparser provides functionality to read and parse BTicino answering machine messages
// from real device filesystem paths using data extracted from ha_config analysis
package messageparser

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"bticino_bridge/pkg/bticino"
)

// MessageInfo represents the structure of msg_info.ini file from real BTicino devices
type MessageInfo struct {
	Date      string `ini:"date"`      // Message date/time
	MediaType string `ini:"mediatype"` // Type of media (0=audio, 1=video)
	EuAddr    string `ini:"euaddr"`    // External unit address
	Cause     string `ini:"cause"`     // Call cause/reason
	Status    string `ini:"status"`    // Message status
	UnixTime  string `ini:"unixtime"`  // Unix timestamp
	Read      string `ini:"read"`      // Read status (0=unread, 1=read)
	Duration  string `ini:"duration"`  // Message duration in seconds
}

// Memo represents a voice or text memo (note) from the device
type Memo struct {
	ID        int       `json:"id"`
	Type      string    `json:"type"` // "voice" or "text"
	Read      bool      `json:"read"`
	Timestamp time.Time `json:"timestamp"`
	Duration  string    `json:"duration"` // For voice memos
	Content   string    `json:"content"`  // For text memos
	AudioPath string    `json:"audio_path,omitempty"`
	TextPath  string    `json:"text_path,omitempty"`
}

// MemoParser handles parsing of BTicino memos (voice and text notes)
type MemoParser struct {
	memosTextDir  string
	memosVoiceDir string
}

// NewMemoParser creates a new memo parser instance
func NewMemoParser() *MemoParser {
	return &MemoParser{
		memosTextDir:  "/home/bticino/cfg/extra/47/memos_text/",
		memosVoiceDir: "/home/bticino/cfg/extra/47/memos_voice/",
	}
}

// GetAllMemos retrieves all memos (voice and text) from the device
func (mp *MemoParser) GetAllMemos() ([]*Memo, error) {
	var memos []*Memo

	// Parse text memos
	textMemos, err := mp.parseMemosDir(mp.memosTextDir, "text")
	if err != nil {
		return nil, err
	}
	memos = append(memos, textMemos...)

	// Parse voice memos
	voiceMemos, err := mp.parseMemosDir(mp.memosVoiceDir, "voice")
	if err != nil {
		return nil, err
	}
	memos = append(memos, voiceMemos...)

	// Sort by timestamp descending
	sort.Slice(memos, func(i, j int) bool {
		return memos[i].Timestamp.After(memos[j].Timestamp)
	})

	return memos, nil
}

func (mp *MemoParser) parseMemosDir(dir string, memoType string) ([]*Memo, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []*Memo{}, nil
	}

	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read memos directory %s: %v", dir, err)
	}

	var memos []*Memo
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "memo_") {
			memoNumStr := strings.TrimPrefix(entry.Name(), "memo_")
			memoNum, err := strconv.Atoi(memoNumStr)
			if err != nil {
				continue
			}

			memo, err := mp.parseMemo(filepath.Join(dir, entry.Name()), memoNum, memoType)
			if err != nil {
				continue
			}
			memos = append(memos, memo)
		}
	}
	return memos, nil
}

func (mp *MemoParser) parseMemo(dir string, id int, memoType string) (*Memo, error) {
	memo := &Memo{
		ID:   id,
		Type: memoType,
	}

	infoPath := filepath.Join(dir, "msg_info.ini")
	if data, err := ioutil.ReadFile(infoPath); err == nil {
		content := string(data)
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Skip section headers like [Message Information]
			if strings.HasPrefix(line, "[") {
				continue
			}
			if strings.HasPrefix(line, "Read=") {
				readVal := strings.TrimPrefix(line, "Read=")
				memo.Read = readVal == "1"
			} else if strings.HasPrefix(line, "Date=") {
				dateStr := strings.TrimPrefix(line, "Date=")
				// Format: "Tue, 3 Oct 2023 15:26:46 +0200"
				parsedTime, err := time.Parse("Mon, 2 Jan 2006 15:04:05 -0700", dateStr)
				if err == nil {
					memo.Timestamp = parsedTime
				}
			} else if strings.HasPrefix(line, "Duration=") {
				memo.Duration = strings.TrimPrefix(line, "Duration=")
			}
		}
	}

	if memoType == "text" {
		txtPath := filepath.Join(dir, "message.txt")
		if data, err := ioutil.ReadFile(txtPath); err == nil {
			memo.Content = strings.TrimSpace(string(data))
			memo.TextPath = txtPath
		}
	} else {
		audioPath := filepath.Join(dir, "audio.wav")
		if _, err := os.Stat(audioPath); err == nil {
			memo.AudioPath = audioPath
		}
	}

	return memo, nil
}

// MarkMemoAsRead marks a memo as read by updating its msg_info.ini file
func (mp *MemoParser) MarkMemoAsRead(memoID int, memoType string) error {
	dirName := fmt.Sprintf("memo_%d", memoID)
	var dir string
	if memoType == "text" {
		dir = filepath.Join(mp.memosTextDir, dirName)
	} else {
		dir = filepath.Join(mp.memosVoiceDir, dirName)
	}

	infoPath := filepath.Join(dir, "msg_info.ini")
	data, err := ioutil.ReadFile(infoPath)
	if err != nil {
		return fmt.Errorf("failed to read memo info file: %v", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	updated := false

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Read=") {
			lines[i] = "Read=1"
			updated = true
			break
		}
	}

	if !updated {
		lines = append(lines, "Read=1")
	}

	newContent := strings.Join(lines, "\n")
	return ioutil.WriteFile(infoPath, []byte(newContent), 0644)
}

// MarkMemoAsUnread marks a memo as unread by updating its msg_info.ini file
func (mp *MemoParser) MarkMemoAsUnread(memoID int, memoType string) error {
	dirName := fmt.Sprintf("memo_%d", memoID)
	var dir string
	if memoType == "text" {
		dir = filepath.Join(mp.memosTextDir, dirName)
	} else {
		dir = filepath.Join(mp.memosVoiceDir, dirName)
	}

	infoPath := filepath.Join(dir, "msg_info.ini")
	data, err := ioutil.ReadFile(infoPath)
	if err != nil {
		return fmt.Errorf("failed to read memo info file: %v", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	updated := false

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Read=") {
			lines[i] = "Read=0"
			updated = true
			break
		}
	}

	if !updated {
		lines = append(lines, "Read=0")
	}

	newContent := strings.Join(lines, "\n")
	return ioutil.WriteFile(infoPath, []byte(newContent), 0644)
}

// DeleteMemo deletes a memo by removing its directory
func (mp *MemoParser) DeleteMemo(memoID int, memoType string) error {
	dirName := fmt.Sprintf("memo_%d", memoID)
	var dir string
	if memoType == "text" {
		dir = filepath.Join(mp.memosTextDir, dirName)
	} else {
		dir = filepath.Join(mp.memosVoiceDir, dirName)
	}

	return os.RemoveAll(dir)
}

// Message represents a complete answering machine message with all data
type Message struct {
	ID          int          `json:"id"`
	MessageInfo *MessageInfo `json:"message_info,omitempty"`
	CallerID    string       `json:"caller_id"`
	Message     string       `json:"message"`
	Type        string       `json:"type"`
	Read        bool         `json:"read"`
	Timestamp   time.Time    `json:"timestamp"`
	Duration    string       `json:"duration"`
	HasImage    bool         `json:"has_image"`
	HasVideo    bool         `json:"has_video"`
	ImagePath   string       `json:"image_path,omitempty"`
	VideoPath   string       `json:"video_path,omitempty"`
	ImageBase64 string       `json:"image_base64,omitempty"`
}

// AnsweringMachineStatus represents the current status of the answering machine system
type AnsweringMachineStatus struct {
	Enabled            bool      `json:"enabled"`
	TotalMessages      int       `json:"total_messages"`
	NewMessages        int       `json:"new_messages"`
	StorageUsed        string    `json:"storage_used"`
	LastChecked        time.Time `json:"last_checked"`
	ServiceAvailable   bool      `json:"service_available"`
	StorageCapacityMB  int       `json:"storage_capacity_mb"`
	StorageUsedMB      int       `json:"storage_used_mb"`
	MaxMessageDuration string    `json:"max_message_duration"`
	RetentionDays      int       `json:"retention_days"`
}

// MessageParser handles parsing of BTicino answering machine messages
type MessageParser struct {
	messagesDir string
}

// NewMessageParser creates a new message parser instance
func NewMessageParser() *MessageParser {
	return &MessageParser{
		messagesDir: bticino.MessagesDir,
	}
}

// SetMessagesDirectory allows overriding the default messages directory (for testing)
func (mp *MessageParser) SetMessagesDirectory(dir string) {
	mp.messagesDir = dir
}

// GetAllMessages retrieves all messages from the BTicino device filesystem
func (mp *MessageParser) GetAllMessages() ([]*Message, error) {
	if _, err := os.Stat(mp.messagesDir); os.IsNotExist(err) {
		return []*Message{}, nil // Return empty slice if directory doesn't exist
	}

	entries, err := ioutil.ReadDir(mp.messagesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read messages directory: %v", err)
	}

	var messages []*Message
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), bticino.MessageDirPattern) {
			messageNumStr := strings.TrimPrefix(entry.Name(), bticino.MessageDirPattern)
			messageNum, err := strconv.Atoi(messageNumStr)
			if err != nil {
				continue // Skip invalid message directories
			}

			message, err := mp.parseMessage(messageNum)
			if err != nil {
				continue // Skip messages that can't be parsed
			}

			messages = append(messages, message)
		}
	}

	// Sort messages by timestamp (newest first)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp.After(messages[j].Timestamp)
	})

	return messages, nil
}

// GetMessage retrieves a specific message by ID
func (mp *MessageParser) GetMessage(messageID int) (*Message, error) {
	return mp.parseMessage(messageID)
}

// parseMessage parses a single message from the filesystem
func (mp *MessageParser) parseMessage(messageID int) (*Message, error) {
	messageDirPath := filepath.Join(mp.messagesDir, fmt.Sprintf("%s%d", bticino.MessageDirPattern, messageID))

	// Check if message directory exists
	if _, err := os.Stat(messageDirPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("message %d not found", messageID)
	}

	message := &Message{
		ID: messageID,
	}

	// Parse msg_info.ini
	infoFilePath := filepath.Join(messageDirPath, bticino.MessageInfoFile)
	if _, err := os.Stat(infoFilePath); err == nil {
		messageInfo, err := mp.parseMessageInfo(infoFilePath)
		if err == nil {
			message.MessageInfo = messageInfo

			// Format duration properly
			if messageInfo.Duration != "" && messageInfo.Duration != "0" {
				if duration, err := strconv.Atoi(messageInfo.Duration); err == nil {
					minutes := duration / 60
					seconds := duration % 60
					message.Duration = fmt.Sprintf("%02d:%02d", minutes, seconds)
				} else {
					message.Duration = messageInfo.Duration + "s"
				}
			} else {
				message.Duration = "00:00"
			}

			message.Read = messageInfo.Read == "1"

			// Parse timestamp from unixtime
			if unixTime, err := strconv.ParseInt(messageInfo.UnixTime, 10, 64); err == nil {
				message.Timestamp = time.Unix(unixTime, 0)
			} else {
				message.Timestamp = time.Now()
			}

			// Determine message type and caller ID based on cause and address
			message.CallerID, message.Type = mp.determineCallerAndType(messageInfo)

			// Generate descriptive message
			message.Message = mp.generateMessageDescription(messageInfo)
		}
	}

	// Check for image file
	imageFilePath := filepath.Join(messageDirPath, bticino.MessageImageFile)
	if _, err := os.Stat(imageFilePath); err == nil {
		message.HasImage = true
		message.ImagePath = imageFilePath

		// Optionally load image as base64 (for web interface)
		if imageData, err := ioutil.ReadFile(imageFilePath); err == nil {
			message.ImageBase64 = base64.StdEncoding.EncodeToString(imageData)
		}
	}

	// Check for video file
	videoFilePath := filepath.Join(messageDirPath, bticino.MessageVideoFile)
	if _, err := os.Stat(videoFilePath); err == nil {
		message.HasVideo = true
		message.VideoPath = videoFilePath
	}

	// Set default values if not parsed from info file
	if message.Timestamp.IsZero() {
		message.Timestamp = time.Now()
	}
	if message.Duration == "" {
		message.Duration = "00:00:00"
	}
	if message.Message == "" {
		message.Message = "Message available"
	}
	if message.CallerID == "" {
		message.CallerID = "Unknown"
	}
	if message.Type == "" {
		message.Type = bticino.MessageTypeVoice
	}

	return message, nil
}

// parseMessageInfo parses the msg_info.ini file using simple text parsing
func (mp *MessageParser) parseMessageInfo(filePath string) (*MessageInfo, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read message info file: %v", err)
	}

	content := string(data)
	messageInfo := &MessageInfo{}

	// Parse key=value pairs from the INI file
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				switch strings.ToLower(key) {
				case "date":
					messageInfo.Date = value
				case "mediatype":
					messageInfo.MediaType = value
				case "euaddr":
					messageInfo.EuAddr = value
				case "cause":
					messageInfo.Cause = value
				case "status":
					messageInfo.Status = value
				case "unixtime":
					messageInfo.UnixTime = value
				case "read":
					messageInfo.Read = value
				case "duration":
					messageInfo.Duration = value
				}
			}
		}
	}

	return messageInfo, nil
}

// determineCallerAndType determines caller ID and message type based on message info
func (mp *MessageParser) determineCallerAndType(info *MessageInfo) (string, string) {
	callerID := "Unknown"
	messageType := bticino.MessageTypeVoice

	// Decode MediaType values from real BTicino device
	switch info.MediaType {
	case "0":
		messageType = bticino.MessageTypeVoice
		callerID = "Audio Call"
	case "1":
		messageType = bticino.MessageTypeVoice // Use voice for video until we add video type
		callerID = "Video Call"
	case "2":
		messageType = bticino.MessageTypeVoice // Use voice for video until we add video type
		callerID = "Video Intercom"
	default:
		if info.MediaType != "" {
			messageType = bticino.MessageTypeSystem
			callerID = "MediaType " + info.MediaType
		}
	}

	// Use EuAddr (External Unit Address) to identify specific intercoms
	if info.EuAddr != "" && info.EuAddr != "0" {
		switch info.EuAddr {
		case "20":
			callerID = "Main Intercom (20)"
		case "21":
			callerID = "Secondary Intercom (21)"
		default:
			callerID = "External Unit " + info.EuAddr
		}
	}

	// Analyze cause field for additional context
	if info.Cause != "" && info.Cause != "0" {
		switch info.Cause {
		case "1":
			callerID += " - Door Bell"
			messageType = bticino.MessageTypeDoor
		case "2":
			callerID += " - System Alert"
			messageType = bticino.MessageTypeSystem
		case "3":
			callerID += " - Missed Call"
		default:
			if info.Cause != "0" {
				callerID += " - Cause " + info.Cause
			}
		}
	}

	return callerID, messageType
}

// generateMessageDescription creates a human-readable message description
func (mp *MessageParser) generateMessageDescription(info *MessageInfo) string {
	// Start with basic description based on MediaType
	var description string
	switch info.MediaType {
	case "0":
		description = "Audio message"
	case "1":
		description = "Video message"
	case "2":
		description = "Video intercom call"
	default:
		description = "Message"
	}

	// Add duration information
	if info.Duration != "" && info.Duration != "0" {
		description += fmt.Sprintf(" (%s seconds)", info.Duration)
	}

	// Add source information
	if info.EuAddr != "" && info.EuAddr != "0" {
		description += fmt.Sprintf(" from unit %s", info.EuAddr)
	}

	// Add cause information
	if info.Cause != "" && info.Cause != "0" {
		switch info.Cause {
		case "1":
			description += " - Door bell activation"
		case "2":
			description += " - System notification"
		case "3":
			description += " - Missed call"
		default:
			description += fmt.Sprintf(" - Event type %s", info.Cause)
		}
	}

	// Add date information if available
	if info.Date != "" {
		description += fmt.Sprintf(" recorded on %s", info.Date)
	}

	return description
}

// GetAnsweringMachineStatus returns the current status of the answering machine
func (mp *MessageParser) GetAnsweringMachineStatus() (*AnsweringMachineStatus, error) {
	messages, err := mp.GetAllMessages()
	if err != nil {
		return nil, fmt.Errorf("failed to get messages for status: %v", err)
	}

	// Calculate statistics
	totalMessages := len(messages)
	newMessages := 0
	for _, msg := range messages {
		if !msg.Read {
			newMessages++
		}
	}

	// Calculate storage usage
	storageUsedMB := mp.calculateStorageUsage()
	storageCapacityMB := 128 // Default BTicino storage capacity
	storageUsedPercent := int((float64(storageUsedMB) / float64(storageCapacityMB)) * 100)

	status := &AnsweringMachineStatus{
		Enabled:            mp.isAnsweringMachineEnabled(),
		TotalMessages:      totalMessages,
		NewMessages:        newMessages,
		StorageUsed:        fmt.Sprintf("%d%%", storageUsedPercent),
		LastChecked:        time.Now(),
		ServiceAvailable:   true,
		StorageCapacityMB:  storageCapacityMB,
		StorageUsedMB:      storageUsedMB,
		MaxMessageDuration: "00:05:00", // 5 minutes typical max
		RetentionDays:      30,
	}

	return status, nil
}

// calculateStorageUsage calculates the total storage used by messages (in MB)
func (mp *MessageParser) calculateStorageUsage() int {
	var totalSize int64

	filepath.Walk(mp.messagesDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	// Convert bytes to MB
	return int(totalSize / (1024 * 1024))
}

// isAnsweringMachineEnabled checks if the answering machine is currently enabled
func (mp *MessageParser) isAnsweringMachineEnabled() bool {
	// Check if the messages directory exists and is accessible
	if _, err := os.Stat(mp.messagesDir); os.IsNotExist(err) {
		return false
	}

	// Check if segreteria temp directory exists (indicates active answering machine)
	if _, err := os.Stat(bticino.AnsweringMachineTemp); err == nil {
		return true
	}

	return true // Default to enabled if messages directory exists
}

// GetImageData returns the image data for a message
func (mp *MessageParser) GetImageData(messageID int) ([]byte, error) {
	messageDirPath := filepath.Join(mp.messagesDir, fmt.Sprintf("%s%d", bticino.MessageDirPattern, messageID))
	imageFilePath := filepath.Join(messageDirPath, bticino.MessageImageFile)

	return ioutil.ReadFile(imageFilePath)
}

// GetVideoData returns the video data for a message
func (mp *MessageParser) GetVideoData(messageID int) ([]byte, error) {
	messageDirPath := filepath.Join(mp.messagesDir, fmt.Sprintf("%s%d", bticino.MessageDirPattern, messageID))
	videoFilePath := filepath.Join(messageDirPath, bticino.MessageVideoFile)

	return ioutil.ReadFile(videoFilePath)
}

// MarkMessageAsRead marks a message as read by updating its msg_info.ini file
func (mp *MessageParser) MarkMessageAsRead(messageID int) error {
	messageDirPath := filepath.Join(mp.messagesDir, fmt.Sprintf("%s%d", bticino.MessageDirPattern, messageID))
	infoFilePath := filepath.Join(messageDirPath, bticino.MessageInfoFile)

	data, err := ioutil.ReadFile(infoFilePath)
	if err != nil {
		return fmt.Errorf("failed to read message info file: %v", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	updated := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "read=") {
			lines[i] = "read=1"
			updated = true
			break
		}
	}

	if !updated {
		lines = append(lines, "read=1")
	}

	newContent := strings.Join(lines, "\n")
	return ioutil.WriteFile(infoFilePath, []byte(newContent), 0644)
}

// MarkMessageAsUnread marks a message as unread by updating its msg_info.ini file
func (mp *MessageParser) MarkMessageAsUnread(messageID int) error {
	messageDirPath := filepath.Join(mp.messagesDir, fmt.Sprintf("%s%d", bticino.MessageDirPattern, messageID))
	infoFilePath := filepath.Join(messageDirPath, bticino.MessageInfoFile)

	data, err := ioutil.ReadFile(infoFilePath)
	if err != nil {
		return fmt.Errorf("failed to read message info file: %v", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	updated := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "read=") {
			lines[i] = "read=0"
			updated = true
			break
		}
	}

	if !updated {
		lines = append(lines, "read=0")
	}

	newContent := strings.Join(lines, "\n")
	return ioutil.WriteFile(infoFilePath, []byte(newContent), 0644)
}

// DeleteMessage removes a message directory and all its contents
func (mp *MessageParser) DeleteMessage(messageID int) error {
	messageDirPath := filepath.Join(mp.messagesDir, fmt.Sprintf("%s%d", bticino.MessageDirPattern, messageID))

	if _, err := os.Stat(messageDirPath); os.IsNotExist(err) {
		return fmt.Errorf("message %d not found", messageID)
	}

	return os.RemoveAll(messageDirPath)
}

// ClearOldMessages removes messages older than the specified number of days
func (mp *MessageParser) ClearOldMessages(olderThanDays int) error {
	messages, err := mp.GetAllMessages()
	if err != nil {
		return err
	}

	cutoffTime := time.Now().AddDate(0, 0, -olderThanDays)
	deletedCount := 0

	for _, message := range messages {
		if message.Timestamp.Before(cutoffTime) {
			if err := mp.DeleteMessage(message.ID); err == nil {
				deletedCount++
			}
		}
	}

	return nil
}
