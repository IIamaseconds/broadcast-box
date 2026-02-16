package session

import (
	"log"
	"time"

	"github.com/glimesh/broadcast-box/internal/webrtc/sessions/whep"
	"github.com/glimesh/broadcast-box/internal/webrtc/sessions/whip"
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

// When WHEP is established, send initial messages to client
func (session *Session) handleWhepConnection(whipSession *whip.WhipSession, whepSession *whep.WhepSession) {
	log.Println("Session.WhepSession.Connected:", session.StreamKey)
	whepSession.SseEventsChannel <- session.GetSessionStatsEvent()
	whepSession.SseEventsChannel <- whipSession.GetAvailableLayersEvent()

	<-whepSession.ActiveContext.Done()

	log.Println("Session.WhepSession.Disconnected:", session.StreamKey, " - ", whepSession.SessionId)
	session.removeWhep(whepSession.SessionId)
}

func (session *Session) handleWhepVideoRtcpSender(whepSession *whep.WhepSession, rtcpSender *webrtc.RTPSender) {
	for {
		rtcpPackets, _, rtcpErr := rtcpSender.ReadRTCP()
		if rtcpErr != nil {
			log.Println("WhepSession.ReadRTCP.Error:", rtcpErr)
			return
		}

		for _, packet := range rtcpPackets {
			if _, isPLI := packet.(*rtcp.PictureLossIndication); isPLI {
				whepSession.SendPLI()
			}
		}
	}
}

// - Initializes by announcing stream start to potentially awaiting clients
// - Announces layers changes to clients when layers are added or removed from the session
// - Triggers a status update every 5 seconds to send to all listening WHEP sessions
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

				session.WhepSessionsLock.RLock()
				for _, whepSession := range session.WhepSessions {
					select {
					case whepSession.SseEventsChannel <- status:
					default:
						log.Println("Session.Host.HostStatusLoop: SSE channel full, skipping", whepSession.SessionId)
					}
				}
				session.WhepSessionsLock.RUnlock()

			}
		}
	}
}

// Start a routing that takes snapshots of the current whep sessions in the whip session.
func (session *Session) Snapshot() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-session.ActiveContext.Done():
			if host := session.Host.Load(); host != nil {
				host.WhepSessionsSnapshot.Store(make(map[string]*whep.WhepSession))
			}
			return
		case <-ticker.C:
			if host := session.Host.Load(); host != nil {
				session.WhepSessionsLock.RLock()
				snapshot := make(map[string]*whep.WhepSession, len(session.WhepSessions))

				for _, whepSession := range session.WhepSessions {
					if !whepSession.IsSessionClosed.Load() {
						snapshot[whepSession.SessionId] = whepSession
					}
				}
				session.WhepSessionsLock.RUnlock()

				host.WhepSessionsSnapshot.Store(snapshot)
			}
		}
	}
}
