package whep

import (
	"context"
	"log"
	"time"

	"github.com/glimesh/broadcast-box/internal/webrtc/codecs"
	"github.com/pion/webrtc/v4"
)

// Create and start a new WHEP session
func CreateNewWHEP(
	whepSessionID string,
	audioTrack *codecs.TrackMultiCodec,
	audioLayer string,
	videoTrack *codecs.TrackMultiCodec,
	videoLayer string,
	peerConnection *webrtc.PeerConnection,
	pliSender func(),
) (whepSession *WHEPSession) {
	log.Println("WHEPSession.CreateNewWHEP", whepSessionID)

	activeContext, activeContextCancel := context.WithCancel(context.Background())
	whepSession = &WHEPSession{
		SessionID:               whepSessionID,
		AudioTrack:              audioTrack,
		VideoTrack:              videoTrack,
		AudioTimestamp:          5000,
		VideoTimestamp:          5000,
		SSESubscribers:          make(map[string]sseSubscriber),
		PeerConnection:          peerConnection,
		ActiveContext:           activeContext,
		ActiveContextCancel:     activeContextCancel,
		pliSender:               pliSender,
		videoBitrateWindowStart: time.Now(),
	}

	log.Println("WHEPSession.CreateNewWHEP.AudioLayer", audioLayer)
	log.Println("WHEPSession.CreateNewWHEP.VideoLayer", videoLayer)
	whepSession.AudioLayerCurrent.Store(audioLayer)
	whepSession.VideoLayerCurrent.Store(videoLayer)
	whepSession.IsWaitingForKeyframe.Store(true)
	whepSession.IsSessionClosed.Store(false)
	return whepSession
}

// Closes down the WHEP session completely
func (whepSession *WHEPSession) Close() {
	// Close WHEP channels
	whepSession.SessionClose.Do(func() {
		log.Println("WHEPSession.Close")
		whepSession.IsSessionClosed.Store(true)

		// Close PeerConnection
		log.Println("WHEPSession.Close.PeerConnection.GracefulClose")
		err := whepSession.PeerConnection.Close()
		if err != nil {
			log.Println("WHEPSession.Close.PeerConnection.Error", err)
		}
		log.Println("WHEPSession.Close.PeerConnection.GracefulClose.Completed")

		// Empty tracks
		whepSession.AudioLock.Lock()
		whepSession.VideoLock.Lock()

		whepSession.AudioTrack = nil
		whepSession.VideoTrack = nil

		whepSession.VideoLock.Unlock()
		whepSession.AudioLock.Unlock()

		whepSession.SSESubscribersLock.Lock()
		whepSession.SSESubscribers = make(map[string]sseSubscriber)
		whepSession.SSESubscribersLock.Unlock()

		whepSession.ActiveContextCancel()
	})
}

// Get the current status of the WHEP session
func (whepSession *WHEPSession) GetWHEPSessionStatus() (state SessionState) {
	whepSession.AudioLock.RLock()
	whepSession.VideoLock.Lock()
	whepSession.updateVideoBitrateLocked(time.Now())

	currentAudioLayer := whepSession.AudioLayerCurrent.Load().(string)
	currentVideoLayer := whepSession.VideoLayerCurrent.Load().(string)

	state = SessionState{
		ID: whepSession.SessionID,

		AudioLayerCurrent:   currentAudioLayer,
		AudioTimestamp:      whepSession.AudioTimestamp,
		AudioPacketsWritten: whepSession.AudioPacketsWritten,
		AudioSequenceNumber: uint64(whepSession.AudioSequenceNumber),

		VideoLayerCurrent:   currentVideoLayer,
		VideoTimestamp:      whepSession.VideoTimestamp,
		VideoBitrate:        whepSession.VideoBitrate.Load(),
		VideoPacketsWritten: whepSession.VideoPacketsWritten,
		VideoPacketsDropped: whepSession.VideoPacketsDropped.Load(),
		VideoSequenceNumber: uint64(whepSession.VideoSequenceNumber),
	}

	whepSession.VideoLock.Unlock()
	whepSession.AudioLock.RUnlock()

	return
}

// Finds the corresponding WHIP session to the WHEP session id and sets the requested audio layer
func (whepSession *WHEPSession) SetAudioLayer(encodingID string) {
	log.Println("Setting Audio Layer")
	whepSession.AudioLayerCurrent.Store(encodingID)
	whepSession.IsWaitingForKeyframe.Store(true)
	whepSession.SendPLI()
}

// Finds the corresponding WHIP session to the WHEP session id and sets the requested video layer
func (whepSession *WHEPSession) SetVideoLayer(encodingID string) {
	log.Println("Setting Video Layer")
	whepSession.VideoLayerCurrent.Store(encodingID)
	whepSession.IsWaitingForKeyframe.Store(true)
	whepSession.SendPLI()
}

func (whepSession *WHEPSession) SendPLI() {
	if whepSession.IsSessionClosed.Load() {
		return
	}

	if whepSession.pliSender != nil {
		whepSession.pliSender()
	}
}

func (whepSession *WHEPSession) AddSSESubscriber(subscriberID string, writeEvent func(string) bool, cancel func()) bool {
	whepSession.SSESubscribersLock.Lock()
	defer whepSession.SSESubscribersLock.Unlock()

	if whepSession.IsSessionClosed.Load() || writeEvent == nil || cancel == nil {
		return false
	}

	whepSession.SSESubscribers[subscriberID] = sseSubscriber{
		writeEvent: writeEvent,
		cancel:     cancel,
	}
	return true
}

func (whepSession *WHEPSession) RemoveSSESubscriber(subscriberID string) {
	whepSession.SSESubscribersLock.Lock()
	delete(whepSession.SSESubscribers, subscriberID)
	whepSession.SSESubscribersLock.Unlock()
}

func (whepSession *WHEPSession) BroadcastSSE(message string) {
	if message == "" || whepSession.IsSessionClosed.Load() {
		return
	}

	whepSession.SSESubscribersLock.RLock()
	subscribers := make(map[string]sseSubscriber, len(whepSession.SSESubscribers))
	for id, subscriber := range whepSession.SSESubscribers {
		subscribers[id] = subscriber
	}
	whepSession.SSESubscribersLock.RUnlock()

	for id, subscriber := range subscribers {
		if !subscriber.writeEvent(message) {
			whepSession.RemoveSSESubscriber(id)
			subscriber.cancel()
		}
	}
}

func (whepSession *WHEPSession) updateVideoBitrateLocked(now time.Time) {
	if whepSession.videoBitrateWindowStart.IsZero() {
		whepSession.videoBitrateWindowStart = now
		return
	}

	elapsed := now.Sub(whepSession.videoBitrateWindowStart)
	if elapsed < time.Second {
		return
	}

	bytesDiff := whepSession.VideoBytesWritten - whepSession.videoBitrateWindowBytes
	if bytesDiff < 0 {
		bytesDiff = 0
	}

	whepSession.VideoBitrate.Store(uint64(float64(bytesDiff) / elapsed.Seconds()))
	whepSession.videoBitrateWindowStart = now
	whepSession.videoBitrateWindowBytes = whepSession.VideoBytesWritten
}
