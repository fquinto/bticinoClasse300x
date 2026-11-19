package mqtt

// entityDecision decide qué hacer con una entidad en la discovery de HA según
// mqtt.entities. Sin lista (nil/vacía) se publica todo con el nombre por defecto.
// Con lista: solo se publican los object_id presentes; un valor no vacío es el
// nombre a usar y "" significa nombre por defecto. Independiente del idioma.
func entityDecision(allow map[string]string, objectID string) (publish bool, name string) {
	if len(allow) == 0 {
		return true, ""
	}
	name, ok := allow[objectID]
	return ok, name
}
