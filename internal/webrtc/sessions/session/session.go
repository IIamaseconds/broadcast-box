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

func (s *Session) UpdateStreamStatus(profile authorization.PublicProfile) {
	s.StatusLock.Lock()

	s.HasHost.Store(true)
	s.MOTD = profile.MOTD
	s.IsPublic = profile.IsPublic

	s.StatusLock.Unlock()
}

// Add WHEP session to existing WHIP session
func (s *Session) AddWHEP(whepSessionID string, peerConnection *webrtc.PeerConnection, audioTrack *codecs.TrackMultiCodec, videoTrack *codecs.TrackMultiCodec, videoRTCPSender *webrtc.RTPSender) (err error) {
	log.Println("WHIPSessionManager.WHIPSession.AddWHEPSession")

	host := s.Host.Load()
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

	s.WHEPSessionsLock.Lock()
	s.WHEPSessions[whepSessionID] = whepSession
	s.WHEPSessionsLock.Unlock()
	s.updateHostWHEPSessionsSnapshot()

	go s.handleWHEPConnection(whepSession)
	go s.handleWHEPVideoRTCPSender(whepSession, videoRTCPSender)

	return nil
}

// Add host
func (s *Session) AddHost(peerConnection *webrtc.PeerConnection) (err error) {
	log.Println("Session.AddHost")

	for {
		host := s.Host.Load()
		if host == nil {
			break
		}

		if host.PeerConnection.ConnectionState() != webrtc.PeerConnectionStateClosed || s.ActiveContext.Err() == nil {
			return fmt.Errorf("session already has a host")
		}

		if s.Host.CompareAndSwap(host, nil) {
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

	host.AddPeerConnection(peerConnection, s.StreamKey)
	if !s.Host.CompareAndSwap(nil, host) {
		host.ActiveContextCancel()
		host.RemovePeerConnection()
		host.RemoveTracks()
		return fmt.Errorf("session already has a host")
	}
	host.WHEPSessionsSnapshot.Store(make(map[string]*whep.WHEPSession))
	s.updateHostWHEPSessionsSnapshot()

	go s.hostStatusLoop()

	return nil
}

func (s *Session) RemoveHost() {

	host := s.Host.Swap(nil)
	if host == nil {
		log.Println("Session.RemoveHost", s.StreamKey, "- No host to remove")
		return
	}

	log.Println("Session.RemoveHost", s.StreamKey)

	host.WHEPSessionsSnapshot.Store(make(map[string]*whep.WHEPSession))
	host.ActiveContextCancel()
	host.RemovePeerConnection()
	host.RemoveTracks()
}

// Remove WHEP session from WHIP session
// In case the WHIP session does not have a host, and no more whep sessions, it will
// be remove from the manager.
func (s *Session) removeWHEP(whepSessionID string) {
	log.Println("Session.RemoveWHEPSession:", s.StreamKey, " - ", whepSessionID)

	s.WHEPSessionsLock.Lock()
	if whepSession, ok := s.WHEPSessions[whepSessionID]; ok {
		whepSession.Close()
		delete(s.WHEPSessions, whepSessionID)
	} else {
		log.Println("Session.RemoveWHEPSession.InvalidSession:", s.StreamKey, " - ", whepSessionID)
	}
	s.WHEPSessionsLock.Unlock()
	s.updateHostWHEPSessionsSnapshot()

	if s.isEmpty() {
		s.close()
	}
}

// Remove all Hosts and clients before closing down session
func (s *Session) close() {
	s.WHEPSessionsLock.Lock()
	whepSessions := make([]*whep.WHEPSession, 0, len(s.WHEPSessions))
	for _, whepSession := range s.WHEPSessions {
		whepSessions = append(whepSessions, whepSession)
	}
	s.WHEPSessions = make(map[string]*whep.WHEPSession)
	s.WHEPSessionsLock.Unlock()

	for _, whepSession := range whepSessions {
		whepSession.Close()
	}
	s.updateHostWHEPSessionsSnapshot()

	s.RemoveHost()

	s.ActiveContextCancel()
}

func (s *Session) Close() {
	log.Println("Session.Close", s.StreamKey)
	s.close()
}

// Returns true is no WHIP tracks are present, and no WHEP sessions are waiting for incoming streams
func (s *Session) isEmpty() bool {
	if s.hasWHEPSessions() {
		log.Println("Session.IsEmpty.HasWHEPSessions (false):", s.StreamKey)
		return false
	}

	if s.isStreaming() {
		log.Println("Session.IsEmpty.IsActive (false):", s.StreamKey)
		return false
	}

	log.Println("Session.IsEmpty (true):", s.StreamKey)
	return true
}

// Returns true if any tracks are available for the session
func (s *Session) isStreaming() bool {

	host := s.Host.Load()
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

func (s *Session) hasWHEPSessions() bool {
	s.WHEPSessionsLock.RLock()
	log.Println("Session.HasWHEPSessions:", len(s.WHEPSessions))

	if len(s.WHEPSessions) == 0 {
		s.WHEPSessionsLock.RUnlock()
		return false
	}

	s.WHEPSessionsLock.RUnlock()
	return true
}

func (s *Session) updateHostWHEPSessionsSnapshot() {
	host := s.Host.Load()
	if host == nil {
		return
	}

	s.WHEPSessionsLock.RLock()
	snapshot := make(map[string]*whep.WHEPSession, len(s.WHEPSessions))
	for _, whepSession := range s.WHEPSessions {
		if !whepSession.IsSessionClosed.Load() {
			snapshot[whepSession.SessionID] = whepSession
		}
	}
	s.WHEPSessionsLock.RUnlock()

	host.WHEPSessionsSnapshot.Store(snapshot)
}

// Get the status of the current session
func (s *Session) GetStreamStatus() (status WHIPSessionStatus) {
	s.WHEPSessionsLock.RLock()
	whepSessionsCount := len(s.WHEPSessions)
	s.WHEPSessionsLock.RUnlock()

	s.StatusLock.RLock()

	status = WHIPSessionStatus{
		StreamKey:   s.StreamKey,
		MOTD:        s.MOTD,
		ViewerCount: whepSessionsCount,
		IsOnline:    s.HasHost.Load(),
		StreamStart: s.StreamStart,
	}

	s.StatusLock.RUnlock()

	return
}
