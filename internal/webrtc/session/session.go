package session

import (
	"context"
	"log"
	"maps"
	"time"

	"github.com/glimesh/broadcast-box/internal/server/authorization"
	"github.com/glimesh/broadcast-box/internal/webrtc/codecs"
	"github.com/glimesh/broadcast-box/internal/webrtc/session/whep"
	"github.com/glimesh/broadcast-box/internal/webrtc/session/whip"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

// Prepare the Whip Session Manager
func (manager *WhipSessionManager) Setup() {
	manager.whipSessions = map[string]*whip.WhipSession{}

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			manager.whipSessionsLock.RLock()
			for _, session := range manager.whipSessions {
				if session.IsEmpty() {
					log.Println("WhipSessionManager.Loop.RemoveEmptySessions")
					manager.RemoveWhipSession(session.StreamKey)
				}
			}
			manager.whipSessionsLock.RUnlock()
		}
	}()

}

// Get Whip stream by stream key
func (manager *WhipSessionManager) GetWhipStream(streamKey string) (session *whip.WhipSession, foundSession bool) {
	manager.whipSessionsLock.RLock()
	stream, ok := manager.whipSessions[streamKey]
	manager.whipSessionsLock.RUnlock()

	return stream, ok
}

func (manager *WhipSessionManager) GetWhipStreamBySessionId(sessionId string) (session *whip.WhipSession, foundSession bool) {
	manager.whipSessionsLock.RLock()
	defer manager.whipSessionsLock.RUnlock()

	for _, whipSession := range manager.whipSessions {
		if sessionId == whipSession.SessionId {
			return whipSession, true
		}
	}

	return nil, false
}

// Find Whep session by session id
func (manager *WhipSessionManager) GetWhepStream(sessionId string) (session *whep.WhepSession, foundSession bool) {
	manager.whipSessionsLock.RLock()
	for _, whipSession := range manager.whipSessions {
		whipSession.WhepSessionsLock.RLock()

		if whepSession, ok := whipSession.WhepSessions[sessionId]; ok {
			whipSession.WhepSessionsLock.RUnlock()
			return whepSession, true
		}
		whipSession.WhepSessionsLock.RUnlock()
	}
	manager.whipSessionsLock.RUnlock()

	return nil, false
}

func (manager *WhipSessionManager) GetWhepStreamBySessionId(sessionId string) (whepSession *whep.WhepSession, ok bool) {
	manager.whipSessionsLock.RLock()
	defer manager.whipSessionsLock.RUnlock()

	for _, whipSession := range manager.whipSessions {
		if whipSession == nil {
			continue
		}

		whipSession.WhepSessionsLock.RLock()
		whepSession, ok = whipSession.WhepSessions[sessionId]
		whipSession.WhepSessionsLock.RUnlock()

		if ok {
			break
		}
	}

	return
}

// Get the stream requested, or create it, and add it to the sessions context
func (manager *WhipSessionManager) GetOrAddStream(profile authorization.PublicProfile, isWhip bool) (*whip.WhipSession, error) {
	log.Println("WhipSessionManager.GetOrAddStream")
	session, ok := manager.GetWhipStream(profile.StreamKey)

	if !ok {
		log.Println("GetOrAddStream.AddWhipSession", profile.StreamKey, "was not found, adding")
		session = manager.AddWhipSession(profile)
	} else if isWhip {
		log.Println("GetOrAddStream.UpdateStreamStatus", profile.StreamKey)
		session.UpdateStreamStatus(profile)
	}

	return session, nil
}

func (manager *WhipSessionManager) UpdateProfile(profile *authorization.PersonalProfile) {
	manager.whipSessionsLock.RLock()
	whipSession, ok := manager.whipSessions[profile.StreamKey]
	manager.whipSessionsLock.RUnlock()

	if ok {
		whipSession.StatusLock.Lock()
		whipSession.MOTD = profile.MOTD
		whipSession.IsPublic = profile.IsPublic
		whipSession.StatusLock.Unlock()
	}
}

func (manager *WhipSessionManager) GetSessionStates(includePrivateStreams bool) (result []StreamSession) {
	log.Println("SessionManager.GetSessionStates: IsAdmin", includePrivateStreams)
	SessionManager.whipSessionsLock.RLock()
	copiedSessions := make(map[string]*whip.WhipSession)
	maps.Copy(copiedSessions, SessionManager.whipSessions)
	SessionManager.whipSessionsLock.RUnlock()

	for _, whipSession := range copiedSessions {
		whipSession.StatusLock.RLock()

		if !includePrivateStreams && !whipSession.IsPublic {
			whipSession.StatusLock.RUnlock()
			continue
		}

		streamSession := StreamSession{
			StreamKey:   whipSession.StreamKey,
			IsPublic:    whipSession.IsPublic,
			MOTD:        whipSession.MOTD,
			Sessions:    []whep.WhepSessionState{},
			VideoTracks: []VideoTrackState{},
			AudioTracks: []AudioTrackState{},
		}

		whipSession.StatusLock.RUnlock()
		whipSession.TracksLock.RLock()

		for _, audioTrack := range whipSession.AudioTracks {
			streamSession.AudioTracks = append(
				streamSession.AudioTracks,
				AudioTrackState{
					Rid:             audioTrack.Rid,
					PacketsReceived: audioTrack.PacketsReceived.Load(),
				})
		}

		for _, videoTrack := range whipSession.VideoTracks {
			var lastKeyFrame time.Time
			if value, ok := videoTrack.LastKeyFrame.Load().(time.Time); ok {
				lastKeyFrame = value
			}

			streamSession.VideoTracks = append(
				streamSession.VideoTracks,
				VideoTrackState{
					Rid:             videoTrack.Rid,
					PacketsReceived: videoTrack.PacketsReceived.Load(),
					LastKeyframe:    lastKeyFrame,
				})
		}

		whipSession.TracksLock.RUnlock()

		whipSession.WhepSessionsLock.RLock()
		for _, whepSession := range whipSession.WhepSessions {
			if !whepSession.IsSessionClosed.Load() {
				streamSession.Sessions = append(streamSession.Sessions, whepSession.GetWhepSessionStatus())
			}
		}
		whipSession.WhepSessionsLock.RUnlock()

		result = append(result, streamSession)
	}

	return
}

// Add new Whip session
func (manager *WhipSessionManager) AddWhipSession(profile authorization.PublicProfile) (whipSession *whip.WhipSession) {
	whipActiveContext, whipActiveContextCancel := context.WithCancel(context.Background())

	whipSession = &whip.WhipSession{
		SessionId: uuid.New().String(),

		StreamKey: profile.StreamKey,
		IsPublic:  profile.IsPublic,
		MOTD:      profile.MOTD,

		ActiveContext:               whipActiveContext,
		ActiveContextCancel:         whipActiveContextCancel,
		PacketLossIndicationChannel: make(chan any, 250),
		OnTrackChangeChannel:        make(chan struct{}, 50),
		EventsChannel:               make(chan any, 50),

		AudioTracks: []*whip.AudioTrack{},
		VideoTracks: []*whip.VideoTrack{},

		WhepSessions: map[string]*whep.WhepSession{},
	}

	manager.whipSessionsLock.Lock()
	manager.whipSessions[profile.StreamKey] = whipSession
	manager.whipSessionsLock.Unlock()

	// Setup Whip session shutdown handling
	go func() {
		<-whipSession.ActiveContext.Done()
		log.Println("WhipSessionManager.WhipSession.ActiveContext.Done()", profile.StreamKey)

		// Remove Whip host
		whipSession.RemovePeerConnection()
		whipSession.RemoveTracks()

		// Remove session if no host or whep sessions are present
		if whipSession.IsEmpty() {
			log.Println("WhipSessionManager.WhipSession.IsEmpty.Remove", profile.StreamKey)
			manager.RemoveWhipSession(profile.StreamKey)
		}
	}()

	return whipSession
}

// Add WHEP session to existing WHIP session
func (manager *WhipSessionManager) AddWhepSession(whepSessionId string, whipSession *whip.WhipSession, peerConnection *webrtc.PeerConnection, audioTrack *codecs.TrackMultiCodec, videoTrack *codecs.TrackMultiCodec) {

	whepSession := whep.CreateNewWhep(
		whepSessionId,
		audioTrack,
		whipSession.GetHighestPrioritizedAudioTrack(),
		videoTrack,
		whipSession.GetHighestPrioritizedVideoTrack(),
		peerConnection)

	whipSession.WhepSessionsLock.Lock()
	whipSession.WhepSessions[whepSessionId] = whepSession
	whipSession.WhepSessionsLock.Unlock()

	// TODO: Move to WhepSession as its own events file
	whepSession.PeerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Println("WhepSession.OnICEConnectionStateChange", state)
		switch state {
		case
			webrtc.ICEConnectionStateConnected:
			// Trigger a new keyframe from the whip session to get feed faster
			log.Println("WhepSession.OnICEConnectionStateChange.Trigger.KeyFrame")
			whipSession.PacketLossIndicationChannel <- true
		case
			webrtc.ICEConnectionStateFailed,
			webrtc.ICEConnectionStateClosed,
			webrtc.ICEConnectionStateDisconnected:
			log.Println("WhepSession.OnICEConnectionStateChange.Trigger.ConnectionState.RemoveWhepSession:", state)
			whipSession.RemoveWhepSession(whepSessionId)
		}
	})

	// When WHEP is established, send initial messages to client
	go func() {
		log.Println("WhipSessionManager.WhepSession.Starting")
		whepSession.SseEventsChannel <- whipSession.GetSessionStatsEvent()
		whepSession.SseEventsChannel <- whipSession.GetAvailableLayersEvent()

		<-whepSession.SessionClosedChannel
		log.Println("WhipSessionManager.WhepSession.Close")
		manager.RemoveWhepSession(whipSession, whepSessionId)
	}()
}

// Remove Whip session completely
func (manager *WhipSessionManager) RemoveWhipSession(streamKey string) {
	log.Println("WhipSessionManager.RemoveWhipSession:", streamKey)
	whipSession, ok := manager.GetWhipStream(streamKey)

	if ok {
		log.Println("WhipSessionManager.RemoveWhipSession.Processing:", streamKey)
		whipSession.RemoveTracks()
		whipSession.RemoveWhepSessions()

		manager.whipSessionsLock.Lock()
		delete(manager.whipSessions, streamKey)
		manager.whipSessionsLock.Unlock()
	} else {
		log.Println("WhipSessionManager.RemoveWhipSession: Could not find", streamKey)
	}
}

// Remove Whep session from Whip session
// In case the Whip session does not have a host, and no more whep sessions, it will
// be remove from the manager.
func (manager *WhipSessionManager) RemoveWhepSession(whipSession *whip.WhipSession, whepSessionId string) {
	log.Println("WhipSessionManager.RemoveWhepSession:", whepSessionId)
	whipSession.WhepSessionsLock.Lock()
	delete(whipSession.WhepSessions, whepSessionId)
	whipSession.WhepSessionsLock.Unlock()

	if whipSession.IsEmpty() {
		log.Println("WhipSessionManager.RemoveWhepSession: WhipSession empty, closing")
		manager.RemoveWhipSession(whipSession.StreamKey)
	}
}
