package session

import (
	"log"

	"github.com/glimesh/broadcast-box/internal/webrtc/utils"
)

// Get SSE String with status about the current session
func (session *Session) GetSessionStatsEvent() string {

	status, err := utils.ToJsonString(session.GetStreamStatus())
	if err != nil {
		log.Println("GetSessionStatsJsonString Error:", err)
		return ""
	}

	return "event: status\ndata: " + status + "\n\n"
}
