package webserver

import (
	"encoding/json"
	"net/http"

	"bticino_bridge/pkg/bticino"
)

// Talk-back (audio ascendente app/bridge → portero). EXPERIMENTAL.
//
// Mecanismo (ver _investigacion/docs/TALKBACK_AUDIO_ASCENDENTE_2026-07-09.md):
// el media de la llamada es RTP plano (el c300x usa crypto "dummy" en el SDP).
// El talk-back = (1) armar el audio bidireccional con el comando OWN
// *8*2#5#4*16## (bticino.CmdAnswerPickup), y (2) enviar RTP Speex/8000 (pt=110)
// al puerto de recepción de audio del portero (SDP-negociado por sesión → se
// pasa como parámetro para poder probar el puerto observado en una llamada viva).
//
// Endpoints:
//   POST /api/talkback/start   {host?, port, source, loop?, arm?}
//   POST /api/talkback/stop
//   GET  /api/talkback/status

func (ws *WebServer) handleTalkbackStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Host   string `json:"host"`   // destino RTP (default 127.0.0.1)
		Port   int    `json:"port"`   // puerto de recepción de audio del portero (obligatorio)
		PT     int    `json:"pt"`     // RTP payload type Speex/8000 (default 98, el del c300x)
		Source string `json:"source"` // ruta a un fichero de audio (WAV, etc.) — obligatorio
		Loop   bool   `json:"loop"`   // repetir el fichero de forma continua
		Arm    *bool  `json:"arm"`    // enviar el comando de armado (default true)
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ws.writeTalkbackJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "cuerpo JSON inválido",
		})
		return
	}
	if body.Port <= 0 || body.Source == "" {
		ws.writeTalkbackJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "se requieren 'port' (>0) y 'source' (ruta a WAV)",
		})
		return
	}

	arm := true
	if body.Arm != nil {
		arm = *body.Arm
	}

	result := map[string]interface{}{}

	// (1) Armar el audio bidireccional (OWN *8*2#5#4*16##)
	if arm {
		if err := ws.bridge.SendOpenWebNetCommand(bticino.CmdAnswerPickup); err != nil {
			ws.logger.WithError(err).Warn("talkback: fallo al enviar el comando de armado")
			result["arm_error"] = err.Error()
		} else {
			result["armed"] = bticino.CmdAnswerPickup
		}
	}

	// (2) Lanzar el pipeline de subida (RTP Speex → puerto del portero)
	if err := ws.talkback.Start(body.Host, body.Port, body.PT, body.Source, body.Loop); err != nil {
		ws.writeTalkbackJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "error": err.Error(), "arm": result,
		})
		return
	}

	result["success"] = true
	result["target"] = ws.talkback.Target()
	result["message"] = "talk-back iniciado (experimental)"
	ws.writeTalkbackJSON(w, http.StatusOK, result)
}

func (ws *WebServer) handleTalkbackStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ws.talkback.Stop()
	ws.writeTalkbackJSON(w, http.StatusOK, map[string]interface{}{
		"success": true, "message": "talk-back detenido",
	})
}

func (ws *WebServer) handleTalkbackStatus(w http.ResponseWriter, r *http.Request) {
	ws.writeTalkbackJSON(w, http.StatusOK, map[string]interface{}{
		"active": ws.talkback.IsActive(),
		"target": ws.talkback.Target(),
	})
}

func (ws *WebServer) writeTalkbackJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
