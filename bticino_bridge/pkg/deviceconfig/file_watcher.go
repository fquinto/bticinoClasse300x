// Package deviceconfig proporciona un watcher para sincronización en tiempo real
package deviceconfig

import (
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// FileWatcher monitorea cambios en archivos de configuración del dispositivo
type FileWatcher struct {
	logger      *logrus.Logger
	publisher   MQTTPublisherInterface
	topicPrefix string
	watchPaths  []string
	stopCh      chan bool
	lastState   map[string]fileState
	throttleDur time.Duration
	lastPublish time.Time
}

// fileState representa el estado de un archivo vigilado
type fileState struct {
	modTime  time.Time
	checksum int64
	size     int64
}

// NewFileWatcher crea un nuevo watcher de archivos
func NewFileWatcher(logger *logrus.Logger, publisher MQTTPublisherInterface, topicPrefix string) *FileWatcher {
	if topicPrefix == "" {
		topicPrefix = "bticino"
	}

	return &FileWatcher{
		logger:      logger,
		publisher:   publisher,
		topicPrefix: topicPrefix,
		watchPaths: []string{
			"/var/tmp/conf.xml",
			"/home/bticino/cfg/extra/47/aswm_settings.ini",
			"/home/bticino/cfg/extra/47/tvcc_settings.ini",
			"/home/bticino/cfg/extra/0/settings.xml",
		},
		stopCh:      make(chan bool),
		lastState:   make(map[string]fileState),
		throttleDur: 2 * time.Second, // Throttle para evitar floods
	}
}

// Start inicia el watcher con polling
func (w *FileWatcher) Start() {
	w.logger.Info("Starting device config file watcher")

	// Inicializar estado
	for _, path := range w.watchPaths {
		if stat, err := os.Stat(path); err == nil {
			w.lastState[path] = fileState{
				modTime:  stat.ModTime(),
				size:     stat.Size(),
				checksum: stat.Size(), // Usar size como checksum simple
			}
		}
	}

	go w.pollLoop()
}

// Stop detiene el watcher
func (w *FileWatcher) Stop() {
	w.stopCh <- true
	w.logger.Info("Device config file watcher stopped")
}

// pollLoop verifica cambios periódicamente
func (w *FileWatcher) pollLoop() {
	ticker := time.NewTicker(2 * time.Second) // Verificar cada 2 segundos
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.checkChanges()
		case <-w.stopCh:
			return
		}
	}
}

// checkChanges verifica si algún archivo ha cambiado
func (w *FileWatcher) checkChanges() {
	if w.publisher == nil || !w.publisher.IsConnected() {
		return
	}

	changed := false

	for _, path := range w.watchPaths {
		stat, err := os.Stat(path)
		if err != nil {
			// Archivo no existe o error
			continue
		}

		currentState := fileState{
			modTime:  stat.ModTime(),
			size:     stat.Size(),
			checksum: stat.Size(),
		}

		lastState, exists := w.lastState[path]
		if !exists || currentState.modTime.After(lastState.modTime) || currentState.size != lastState.size {
			// Hay cambio
			w.lastState[path] = currentState
			changed = true
			w.logger.Infof("Config file changed: %s", path)
		}
	}

	// Throttle: no publicar más de una vez cada throttleDur
	if changed && time.Since(w.lastPublish) > w.throttleDur {
		w.publishConfigChange()
		w.lastPublish = time.Now()
	}
}

// publishConfigChange publica la configuración actualizada
func (w *FileWatcher) publishConfigChange() {
	w.logger.Info("Publishing config change due to file modification")

	paths := struct {
		ConfXML      string
		ASWMSettings string
		TVCCSettings string
		SettingsXML  string
	}{
		ConfXML:      "/var/tmp/conf.xml",
		ASWMSettings: "/home/bticino/cfg/extra/47/aswm_settings.ini",
		TVCCSettings: "/home/bticino/cfg/extra/47/tvcc_settings.ini",
		SettingsXML:  "/home/bticino/cfg/extra/0/settings.xml",
	}

	cfg, err := ReadConfigFromPaths(paths)
	if err != nil {
		w.logger.WithError(err).Error("Failed to read config after change")
		return
	}

	// Publicar inmediatamente (sin retain para eventos de cambio)
	w.publishSystemChange(cfg)
	w.publishAnsweringChange(cfg)
	w.publishAudioChange(cfg)
	w.publishDisplayChange(cfg)

	// Publicar topic de notificación de cambio
	w.publisher.Publish(w.topicPrefix+"/system/config_changed", time.Now().Format(time.RFC3339), false)

	w.logger.Info("Config change published successfully")
}

func (w *FileWatcher) publishSystemChange(cfg *Config) {
	prefix := w.topicPrefix + "/system"
	w.publisher.Publish(prefix+"/language", fmt.Sprintf(`{"language":"%s","change":"file_watcher"}`, cfg.Language), false)
	w.publisher.Publish(prefix+"/timezone", fmt.Sprintf(`{"timezone":"%s","change":"file_watcher"}`, cfg.Timezone), false)
}

func (w *FileWatcher) publishAnsweringChange(cfg *Config) {
	prefix := w.topicPrefix + "/answering"
	activated := "false"
	if cfg.Answering.Activated {
		activated = "true"
	}
	w.publisher.Publish(prefix+"/state", fmt.Sprintf(`{"activated":%s,"change":"file_watcher"}`, activated), false)
}

func (w *FileWatcher) publishAudioChange(cfg *Config) {
	prefix := w.topicPrefix + "/audio"
	w.publisher.Publish(prefix+"/ringtone/s0", fmt.Sprintf(`{"value":%d,"change":"file_watcher"}`, cfg.Ringtones.S0), false)
	w.publisher.Publish(prefix+"/volume/s0", fmt.Sprintf(`{"value":%d,"change":"file_watcher"}`, cfg.Volumes.S0), false)
	w.publisher.Publish(prefix+"/volume/door", fmt.Sprintf(`{"value":%d,"change":"file_watcher"}`, cfg.Volumes.Door), false)
}

func (w *FileWatcher) publishDisplayChange(cfg *Config) {
	prefix := w.topicPrefix + "/display"
	w.publisher.Publish(prefix+"/brightness", fmt.Sprintf(`{"value":%d,"change":"file_watcher"}`, cfg.Display.Brightness), false)
}

// GetWatchedFiles retorna los archivos que se están vigilando
func (w *FileWatcher) GetWatchedFiles() []string {
	return w.watchPaths
}

// VerifyFileWatcher verifica la estructura del watcher (para testing)
func VerifyFileWatcher() {
	fmt.Println("Device Config File Watcher")
	fmt.Println("- Watch interval: 2 seconds")
	fmt.Println("- Throttle: 2 seconds")
	fmt.Println("- Watched files:")
	fmt.Println("  * /var/tmp/conf.xml")
	fmt.Println("  * /home/bticino/cfg/extra/47/aswm_settings.ini")
	fmt.Println("  * /home/bticino/cfg/extra/47/tvcc_settings.ini")
	fmt.Println("  * /home/bticino/cfg/extra/0/settings.xml")
	fmt.Println("- Change notification topic: bticino/system/config_changed")
}
