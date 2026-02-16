package whep

import (
	"context"
	"log"
	"time"

	"github.com/glimesh/broadcast-box/internal/webrtc/codecs"
	"github.com/pion/webrtc/v4"
)

// Create and start a new WHEP session
func CreateNewWhep(
	whepSessionId string,
	audioTrack *codecs.TrackMultiCodec,
	audioLayer string,
	videoTrack *codecs.TrackMultiCodec,
	videoLayer string,
	peerConnection *webrtc.PeerConnection,
	pliSender func(),
) (whepSession *WhepSession) {
	log.Println("WhepSession.CreateNewWhep", whepSessionId)

	activeContext, activeContextCancel := context.WithCancel(context.Background())
	whepSession = &WhepSession{
		SessionId:               whepSessionId,
		AudioTrack:              audioTrack,
		VideoTrack:              videoTrack,
		AudioTimestamp:          5000,
		VideoTimestamp:          5000,
		SseSubscribers:          make(map[string]sseSubscriber),
		PeerConnection:          peerConnection,
		ActiveContext:           activeContext,
		ActiveContextCancel:     activeContextCancel,
		pliSender:               pliSender,
		videoBitrateWindowStart: time.Now(),
	}

	log.Println("WhepSession.CreateNewWhep.AudioLayer", audioLayer)
	log.Println("WhepSession.CreateNewWhep.VideoLayer", videoLayer)
	whepSession.AudioLayerCurrent.Store(audioLayer)
	whepSession.VideoLayerCurrent.Store(videoLayer)
	whepSession.IsWaitingForKeyframe.Store(true)
	whepSession.IsSessionClosed.Store(false)
	return whepSession
}

// Closes down the WHEP session completely
func (whepSession *WhepSession) Close() {
	// Close WHEP channels
	whepSession.SessionClose.Do(func() {
		log.Println("WhepSession.Close")
		whepSession.IsSessionClosed.Store(true)

		// Close PeerConnection
		log.Println("WhepSession.Close.PeerConnection.GracefulClose")
		err := whepSession.PeerConnection.Close()
		if err != nil {
			log.Println("WhepSession.Close.PeerConnection.Error", err)
		}
		log.Println("WhepSession.Close.PeerConnection.GracefulClose.Completed")

		// Empty tracks
		whepSession.AudioLock.Lock()
		whepSession.VideoLock.Lock()

		whepSession.AudioTrack = nil
		whepSession.VideoTrack = nil

		whepSession.VideoLock.Unlock()
		whepSession.AudioLock.Unlock()

		whepSession.SseSubscribersLock.Lock()
		whepSession.SseSubscribers = make(map[string]sseSubscriber)
		whepSession.SseSubscribersLock.Unlock()

		whepSession.ActiveContextCancel()
	})
}

// Get the current status of the WHEP session
func (whepSession *WhepSession) GetWhepSessionStatus() (state WhepSessionStateDto) {
	whepSession.AudioLock.RLock()
	whepSession.VideoLock.Lock()
	whepSession.updateVideoBitrateLocked(time.Now())

	currentAudioLayer := whepSession.AudioLayerCurrent.Load().(string)
	currentVideoLayer := whepSession.VideoLayerCurrent.Load().(string)

	state = WhepSessionStateDto{
		Id: whepSession.SessionId,

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

// Finds the corresponding Whip session to the Whep session id and sets the requested audio layer
func (whepSession *WhepSession) SetAudioLayer(encodingId string) {
	log.Println("Setting Audio Layer")
	whepSession.AudioLayerCurrent.Store(encodingId)
	whepSession.IsWaitingForKeyframe.Store(true)
	whepSession.SendPLI()
}

// Finds the corresponding Whip session to the Whep session id and sets the requested video layer
func (whepSession *WhepSession) SetVideoLayer(encodingId string) {
	log.Println("Setting Video Layer")
	whepSession.VideoLayerCurrent.Store(encodingId)
	whepSession.IsWaitingForKeyframe.Store(true)
	whepSession.SendPLI()
}

func (whepSession *WhepSession) SendPLI() {
	if whepSession.IsSessionClosed.Load() {
		return
	}

	if whepSession.pliSender != nil {
		whepSession.pliSender()
	}
}

func (whepSession *WhepSession) AddSSESubscriber(subscriberID string, writeEvent func(string) bool, cancel func()) bool {
	whepSession.SseSubscribersLock.Lock()
	defer whepSession.SseSubscribersLock.Unlock()

	if whepSession.IsSessionClosed.Load() || writeEvent == nil || cancel == nil {
		return false
	}

	whepSession.SseSubscribers[subscriberID] = sseSubscriber{
		writeEvent: writeEvent,
		cancel:     cancel,
	}
	return true
}

func (whepSession *WhepSession) RemoveSSESubscriber(subscriberID string) {
	whepSession.SseSubscribersLock.Lock()
	delete(whepSession.SseSubscribers, subscriberID)
	whepSession.SseSubscribersLock.Unlock()
}

func (whepSession *WhepSession) BroadcastSSE(message string) {
	if message == "" || whepSession.IsSessionClosed.Load() {
		return
	}

	whepSession.SseSubscribersLock.RLock()
	subscribers := make(map[string]sseSubscriber, len(whepSession.SseSubscribers))
	for id, subscriber := range whepSession.SseSubscribers {
		subscribers[id] = subscriber
	}
	whepSession.SseSubscribersLock.RUnlock()

	for id, subscriber := range subscribers {
		if !subscriber.writeEvent(message) {
			whepSession.RemoveSSESubscriber(id)
			subscriber.cancel()
		}
	}
}

func (whepSession *WhepSession) updateVideoBitrateLocked(now time.Time) {
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
