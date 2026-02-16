package session

import (
	"log"

	"github.com/glimesh/broadcast-box/internal/webrtc/sessions/whep"
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

// Send out an event to all WHEP sessions to notify that available layers has changed
func (session *Session) AnnounceStreamStartToWhepClients() {
	log.Println("Session.AnnounceStreamStartToWhepClients:", session.StreamKey)

	session.WhepSessionsLock.RLock()
	whepSessions := make([]*whep.WhepSession, 0, len(session.WhepSessions))
	for _, whepSession := range session.WhepSessions {
		whepSessions = append(whepSessions, whepSession)
	}
	session.WhepSessionsLock.RUnlock()

	streamStartMessage := "event: streamStart\ndata:\n"

	for _, whepSession := range whepSessions {
		if !whepSession.IsSessionClosed.Load() {
			whepSession.BroadcastSSE(streamStartMessage)
		}
	}
}
