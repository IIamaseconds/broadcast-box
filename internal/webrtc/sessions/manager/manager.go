package manager

import (
	"context"
	"log"
	"maps"
	"time"

	"github.com/glimesh/broadcast-box/internal/server/authorization"
	"github.com/glimesh/broadcast-box/internal/webrtc/sessions/session"
	"github.com/glimesh/broadcast-box/internal/webrtc/sessions/whep"
	"github.com/glimesh/broadcast-box/internal/webrtc/sessions/whip"
)

// Prepare the WHIP Session Manager
func (manager *SessionManager) Setup() {
	log.Println("WHIPSessionManager.Setup")

	manager.sessions = make(map[string]*session.Session)
}

// Add new session
func (manager *SessionManager) addSession(profile authorization.PublicProfile) (s *session.Session, err error) {
	log.Println("SessionManager.AddWHIPSession")
	activeContext, activeContextCancel := context.WithCancel(context.Background())

	s = &session.Session{

		StreamKey:   profile.StreamKey,
		IsPublic:    profile.IsPublic,
		MOTD:        profile.MOTD,
		StreamStart: time.Now(),

		ActiveContext:       activeContext,
		ActiveContextCancel: activeContextCancel,

		WHEPSessions: map[string]*whep.WHEPSession{},
	}

	s.HasHost.Store(true)
	manager.sessionsLock.Lock()
	manager.sessions[profile.StreamKey] = s
	manager.sessionsLock.Unlock()

	go func() {
		<-activeContext.Done()
		log.Println("SessionManager.Session.Done")

		manager.sessionsLock.Lock()
		delete(manager.sessions, profile.StreamKey)
		manager.sessionsLock.Unlock()

	}()

	return s, nil
}

// Get the stream requested, or create it, and add it to the sessions context
func (manager *SessionManager) GetOrAddSession(profile authorization.PublicProfile, isWHIP bool) (session *session.Session, err error) {
	session, ok := manager.GetSessionByID(profile.StreamKey)

	if !ok {
		log.Println("SessionManager.GetOrAddStream: Adding", profile.StreamKey)
		session, err = manager.addSession(profile)
	} else if isWHIP {
		log.Println("SessionManager.GetOrAddStream: Updating", profile.StreamKey)
		session.UpdateStreamStatus(profile)
	}

	return session, err
}

// Get Session by id
func (manager *SessionManager) GetSessionByID(streamKey string) (session *session.Session, foundSession bool) {
	log.Println("SessionManager.GetSessionByID", streamKey)

	manager.sessionsLock.RLock()
	defer manager.sessionsLock.RUnlock()

	session, foundSession = manager.sessions[streamKey]
	return session, foundSession
}

// Gets the current state of all sessions
func (manager *SessionManager) GetSessionStates(includePrivateStreams bool) (result []session.StreamSessionState) {
	log.Println("SessionManager.GetSessionStates: IsAdmin", includePrivateStreams)
	manager.sessionsLock.RLock()
	copiedSessions := make(map[string]*session.Session)
	maps.Copy(copiedSessions, manager.sessions)
	manager.sessionsLock.RUnlock()

	for _, s := range copiedSessions {
		s.StatusLock.RLock()

		if !includePrivateStreams && !s.IsPublic {
			s.StatusLock.RUnlock()
			continue
		}

		streamSession := session.StreamSessionState{
			StreamKey:   s.StreamKey,
			StreamStart: s.StreamStart,
			IsPublic:    s.IsPublic,
			MOTD:        s.MOTD,
			Sessions:    []whep.SessionState{},
			VideoTracks: []session.VideoTrackState{},
			AudioTracks: []session.AudioTrackState{},
		}

		s.StatusLock.RUnlock()

		host := s.Host.Load()
		if host != nil {
			host.TracksLock.RLock()

			for _, audioTrack := range host.AudioTracks {
				streamSession.AudioTracks = append(
					streamSession.AudioTracks,
					session.AudioTrackState{
						Rid:             audioTrack.Rid,
						PacketsReceived: audioTrack.PacketsReceived.Load(),
						PacketsDropped:  audioTrack.PacketsDropped.Load(),
					})
			}

			for _, videoTrack := range host.VideoTracks {
				var lastKeyFrame time.Time
				if value, ok := videoTrack.LastKeyFrame.Load().(time.Time); ok {
					lastKeyFrame = value
				}

				streamSession.VideoTracks = append(
					streamSession.VideoTracks,
					session.VideoTrackState{
						Rid:             videoTrack.Rid,
						Bitrate:         videoTrack.Bitrate.Load(),
						PacketsReceived: videoTrack.PacketsReceived.Load(),
						PacketsDropped:  videoTrack.PacketsDropped.Load(),
						LastKeyframe:    lastKeyFrame,
					})
			}

			host.TracksLock.RUnlock()
		}

		s.WHEPSessionsLock.RLock()
		for _, whep := range s.WHEPSessions {
			if !whep.IsSessionClosed.Load() {
				streamSession.Sessions = append(streamSession.Sessions, whep.GetWHEPSessionStatus())
			}
		}
		s.WHEPSessionsLock.RUnlock()

		result = append(result, streamSession)
	}

	return
}

// Update the provided session information
func (manager *SessionManager) UpdateProfile(profile *authorization.PersonalProfile) {
	log.Println("WHIPSessionManager.UpdateProfile")
	manager.sessionsLock.RLock()
	whipSession, ok := manager.sessions[profile.StreamKey]
	manager.sessionsLock.RUnlock()

	if ok {
		whipSession.StatusLock.Lock()
		whipSession.MOTD = profile.MOTD
		whipSession.IsPublic = profile.IsPublic
		whipSession.StatusLock.Unlock()
	}
}

// Get Session by id
func (manager *SessionManager) GetWHEPSessionByID(sessionID string) (whep *whep.WHEPSession, foundSession bool) {
	_, whepSession, foundSession := manager.GetSessionAndWHEPByID(sessionID)
	return whepSession, foundSession
}

func (manager *SessionManager) GetSessionAndWHEPByID(sessionID string) (streamSession *session.Session, whepSession *whep.WHEPSession, foundSession bool) {
	manager.sessionsLock.RLock()
	defer manager.sessionsLock.RUnlock()

	for _, session := range manager.sessions {
		session.WHEPSessionsLock.RLock()
		whepSession, ok := session.WHEPSessions[sessionID]
		session.WHEPSessionsLock.RUnlock()
		if ok {
			return session, whepSession, true
		}
	}

	return nil, nil, false
}

func (manager *SessionManager) GetHostSessionByID(sessionID string) (host *whip.WHIPSession, foundSession bool) {
	manager.sessionsLock.RLock()
	defer manager.sessionsLock.RUnlock()

	for _, session := range manager.sessions {
		host := session.Host.Load()
		if host == nil {
			continue
		}

		if sessionID == host.ID {
			return host, true
		}
	}

	return nil, false
}

func (manager *SessionManager) GetSessionByHostSessionID(sessionID string) (session *session.Session, foundSession bool) {
	manager.sessionsLock.RLock()
	defer manager.sessionsLock.RUnlock()

	for _, session := range manager.sessions {
		host := session.Host.Load()
		if host == nil {
			continue
		}

		if sessionID == host.ID {
			return session, true
		}
	}

	return nil, false
}
