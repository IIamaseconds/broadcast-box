package whip

import "log"

// Remove all WHEP sessions from the WHIP session.
func (whipSession *WhipSession) RemoveWhepSessions() {
	whipSession.WhepSessionsLock.Lock()

	for _, whepSession := range whipSession.WhepSessions {
		whepSession.Close()
		delete(whipSession.WhepSessions, whepSession.SessionId)
	}

	whipSession.WhepSessionsLock.Unlock()
}

// Remove a WHEP session from the WHIP session.
// If the WHIP session no longer has a host, or any WHEP sessions, terminate it.
func (whipSession *WhipSession) RemoveWhepSession(whepSessionId string) {
	log.Println("WhipSession.RemoveWhepSession:", whepSessionId)
	whipSession.WhepSessionsLock.Lock()

	if whepSession, ok := whipSession.WhepSessions[whepSessionId]; ok {
		// Close out Whep session and remove
		whepSession.Close()
		delete(whipSession.WhepSessions, whepSessionId)
	}

	whipSession.WhepSessionsLock.Unlock()

	if whipSession.IsEmpty() {
		log.Println("WhipSession.RemoveWhepSession.Concluded:", whipSession.StreamKey)
		whipSession.ActiveContextCancel()
	}
}
