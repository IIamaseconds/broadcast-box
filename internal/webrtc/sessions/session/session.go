package session

import (
	"context"
	"fmt"
	"log"

	"github.com/glimesh/broadcast-box/internal/server/authorization"
	"github.com/glimesh/broadcast-box/internal/webrtc/codecs"
	"github.com/glimesh/broadcast-box/internal/webrtc/sessions/whep"
	"github.com/glimesh/broadcast-box/internal/webrtc/sessions/whip"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

func (session *Session) UpdateStreamStatus(profile authorization.PublicProfile) {
	session.StatusLock.Lock()

	session.HasHost.Store(true)
	session.MOTD = profile.MOTD
	session.IsPublic = profile.IsPublic

	session.StatusLock.Unlock()
}

// Add WHEP session to existing WHIP session
func (session *Session) AddWHEP(whepSessionID string, peerConnection *webrtc.PeerConnection, audioTrack *codecs.TrackMultiCodec, videoTrack *codecs.TrackMultiCodec, videoRTCPSender *webrtc.RTPSender) (err error) {
	log.Println("WHIPSessionManager.WHIPSession.AddWHEPSession")

	host := session.Host.Load()
	if host == nil {
		return fmt.Errorf("no host was found on the current session")
	}

	whepSession := whep.CreateNewWHEP(
		whepSessionID,
		audioTrack,
		host.GetHighestPrioritizedAudioTrack(),
		videoTrack,
		host.GetHighestPrioritizedVideoTrack(),
		peerConnection,
		host.SendPLI)

	whepSession.RegisterWHEPHandlers(peerConnection)

	session.WHEPSessionsLock.Lock()
	session.WHEPSessions[whepSessionID] = whepSession
	session.WHEPSessionsLock.Unlock()
	session.updateHostWHEPSessionsSnapshot()

	go session.handleWHEPConnection(whepSession)
	go session.handleWHEPVideoRTCPSender(whepSession, videoRTCPSender)

	return nil
}

// Add host
func (session *Session) AddHost(peerConnection *webrtc.PeerConnection) (err error) {
	log.Println("Session.AddHost")

	for {
		host := session.Host.Load()
		if host == nil {
			break
		}

		if host.PeerConnection.ConnectionState() != webrtc.PeerConnectionStateClosed || session.ActiveContext.Err() == nil {
			return fmt.Errorf("session already has a host")
		}

		if session.Host.CompareAndSwap(host, nil) {
			break
		}
	}

	activeContext, activeContextCancel := context.WithCancel(context.Background())

	host := &whip.WHIPSession{
		ID:          uuid.New().String(),
		AudioTracks: make(map[string]*whip.AudioTrack),
		VideoTracks: make(map[string]*whip.VideoTrack),

		ActiveContext:       activeContext,
		ActiveContextCancel: activeContextCancel,
	}

	host.AddPeerConnection(peerConnection, session.StreamKey)
	if !session.Host.CompareAndSwap(nil, host) {
		host.ActiveContextCancel()
		host.RemovePeerConnection()
		host.RemoveTracks()
		return fmt.Errorf("session already has a host")
	}
	host.WHEPSessionsSnapshot.Store(make(map[string]*whep.WHEPSession))
	session.updateHostWHEPSessionsSnapshot()

	go session.hostStatusLoop()

	return nil
}

func (session *Session) RemoveHost() {

	host := session.Host.Swap(nil)
	if host == nil {
		log.Println("Session.RemoveHost", session.StreamKey, "- No host to remove")
		return
	}

	log.Println("Session.RemoveHost", session.StreamKey)

	host.WHEPSessionsSnapshot.Store(make(map[string]*whep.WHEPSession))
	host.ActiveContextCancel()
	host.RemovePeerConnection()
	host.RemoveTracks()
}

// Remove WHEP session from WHIP session
// In case the WHIP session does not have a host, and no more whep sessions, it will
// be remove from the manager.
func (session *Session) removeWHEP(whepSessionID string) {
	log.Println("Session.RemoveWHEPSession:", session.StreamKey, " - ", whepSessionID)

	session.WHEPSessionsLock.Lock()
	if whepSession, ok := session.WHEPSessions[whepSessionID]; ok {
		whepSession.Close()
		delete(session.WHEPSessions, whepSessionID)
	} else {
		log.Println("Session.RemoveWHEPSession.InvalidSession:", session.StreamKey, " - ", whepSessionID)
	}
	session.WHEPSessionsLock.Unlock()
	session.updateHostWHEPSessionsSnapshot()

	if session.isEmpty() {
		session.close()
	}
}

// Remove all Hosts and clients before closing down session
func (session *Session) close() {
	session.WHEPSessionsLock.Lock()
	whepSessions := make([]*whep.WHEPSession, 0, len(session.WHEPSessions))
	for _, whepSession := range session.WHEPSessions {
		whepSessions = append(whepSessions, whepSession)
	}
	session.WHEPSessions = make(map[string]*whep.WHEPSession)
	session.WHEPSessionsLock.Unlock()

	for _, whepSession := range whepSessions {
		whepSession.Close()
	}
	session.updateHostWHEPSessionsSnapshot()

	session.RemoveHost()

	session.ActiveContextCancel()
}

func (session *Session) Close() {
	log.Println("Session.Close", session.StreamKey)
	session.close()
}

// Returns true is no WHIP tracks are present, and no WHEP sessions are waiting for incoming streams
func (session *Session) isEmpty() bool {
	if session.hasWHEPSessions() {
		log.Println("Session.IsEmpty.HasWHEPSessions (false):", session.StreamKey)
		return false
	}

	if session.isStreaming() {
		log.Println("Session.IsEmpty.IsActive (false):", session.StreamKey)
		return false
	}

	log.Println("Session.IsEmpty (true):", session.StreamKey)
	return true
}

// Returns true if any tracks are available for the session
func (session *Session) isStreaming() bool {

	host := session.Host.Load()
	if host == nil {
		return false
	}

	host.TracksLock.RLock()

	if len(host.AudioTracks) != 0 {
		log.Println("Session.IsActive.AudioTracks", len(host.AudioTracks))
		host.TracksLock.RUnlock()
		return true
	}
	if len(host.VideoTracks) != 0 {
		log.Println("Session.IsActive.VideoTracks", len(host.VideoTracks))
		host.TracksLock.RUnlock()
		return true
	}

	host.TracksLock.RUnlock()
	return false
}

func (session *Session) hasWHEPSessions() bool {
	session.WHEPSessionsLock.RLock()
	log.Println("Session.HasWHEPSessions:", len(session.WHEPSessions))

	if len(session.WHEPSessions) == 0 {
		session.WHEPSessionsLock.RUnlock()
		return false
	}

	session.WHEPSessionsLock.RUnlock()
	return true
}

func (session *Session) updateHostWHEPSessionsSnapshot() {
	host := session.Host.Load()
	if host == nil {
		return
	}

	session.WHEPSessionsLock.RLock()
	snapshot := make(map[string]*whep.WHEPSession, len(session.WHEPSessions))
	for _, whepSession := range session.WHEPSessions {
		if !whepSession.IsSessionClosed.Load() {
			snapshot[whepSession.SessionID] = whepSession
		}
	}
	session.WHEPSessionsLock.RUnlock()

	host.WHEPSessionsSnapshot.Store(snapshot)
}

// Get the status of the current session
func (session *Session) GetStreamStatus() (status WHIPSessionStatus) {
	session.WHEPSessionsLock.RLock()
	whepSessionsCount := len(session.WHEPSessions)
	session.WHEPSessionsLock.RUnlock()

	session.StatusLock.RLock()

	status = WHIPSessionStatus{
		StreamKey:   session.StreamKey,
		MOTD:        session.MOTD,
		ViewerCount: whepSessionsCount,
		IsOnline:    session.HasHost.Load(),
		StreamStart: session.StreamStart,
	}

	session.StatusLock.RUnlock()

	return
}
