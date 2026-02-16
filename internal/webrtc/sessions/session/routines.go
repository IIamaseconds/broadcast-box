package session

import (
	"log"
	"time"

	"github.com/glimesh/broadcast-box/internal/webrtc/sessions/whep"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

//TODO: Might not neccessary
// Triggered when a host is disconnected
// func (session *Session) handleHostDisconnect() {
// 	log.Println("Session.Host.Disconnected", session.StreamKey)
//
// 	// WHIP host offline
// 	if session.Host != nil {
// 		session.Host.RemovePeerConnection()
// 		session.Host.RemoveTracks()
// 	}
// 	session.handleAnnounceOffline()
//
// }

// Waits for WHEP disconnect and removes the session
func (session *Session) handleWHEPConnection(whepSession *whep.WHEPSession) {
	log.Println("Session.WHEPSession.Connected:", session.StreamKey)

	<-whepSession.ActiveContext.Done()

	log.Println("Session.WHEPSession.Disconnected:", session.StreamKey, " - ", whepSession.SessionID)
	session.removeWHEP(whepSession.SessionID)
}

func (session *Session) handleWHEPVideoRTCPSender(whepSession *whep.WHEPSession, rtcpSender *webrtc.RTPSender) {
	for {
		rtcpPackets, _, rtcpErr := rtcpSender.ReadRTCP()
		if rtcpErr != nil {
			log.Println("WHEPSession.ReadRTCP.Error:", rtcpErr)
			return
		}

		for _, packet := range rtcpPackets {
			if _, isPLI := packet.(*rtcp.PictureLossIndication); isPLI {
				whepSession.SendPLI()
			}
		}
	}
}

// Broadcast stream status to connected WHEP clients while host is active.
func (session *Session) hostStatusLoop() {
	log.Println("Session.Host.HostStatusLoop")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		host := session.Host.Load()
		if host == nil {
			if session.isEmpty() {
				session.close()
				return
			}

			time.Sleep(5 * time.Second)
			continue
		}

		select {

		case <-host.ActiveContext.Done():
			session.RemoveHost()

			if session.isEmpty() {
				session.close()
			}
			return

		// Send status every 5 seconds
		case <-ticker.C:
			if session.isEmpty() {
				session.close()
			} else if session.Host.Load() != nil {
				status := session.GetSessionStatsEvent()

				session.WHEPSessionsLock.RLock()
				whepSessions := make([]*whep.WHEPSession, 0, len(session.WHEPSessions))
				for _, whepSession := range session.WHEPSessions {
					whepSessions = append(whepSessions, whepSession)
				}
				session.WHEPSessionsLock.RUnlock()

				for _, whepSession := range whepSessions {
					whepSession.BroadcastSSE(status)
				}
			}
		}
	}
}
