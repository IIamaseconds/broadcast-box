package whep

import (
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
) (w *WHEPSession) {
	log.Println("WHEPSession.CreateNewWHEP", whepSessionID)

	w = &WHEPSession{
		SessionID:               whepSessionID,
		AudioTrack:              audioTrack,
		VideoTrack:              videoTrack,
		AudioTimestamp:          5000,
		VideoTimestamp:          5000,
		SSESubscribers:          make(map[string]sseSubscriber),
		PeerConnection:          peerConnection,
		pliSender:               pliSender,
		videoBitrateWindowStart: time.Now(),
	}

	log.Println("WHEPSession.CreateNewWHEP.AudioLayer", audioLayer)
	log.Println("WHEPSession.CreateNewWHEP.VideoLayer", videoLayer)
	w.AudioLayerCurrent.Store(audioLayer)
	w.VideoLayerCurrent.Store(videoLayer)
	w.IsWaitingForKeyframe.Store(true)
	w.IsSessionClosed.Store(false)
	return w
}

// Closes down the WHEP session completely
func (w *WHEPSession) Close() {
	// Close WHEP channels
	w.SessionClose.Do(func() {
		log.Println("WHEPSession.Close")
		w.IsSessionClosed.Store(true)

		// Close PeerConnection
		log.Println("WHEPSession.Close.PeerConnection.GracefulClose")
		err := w.PeerConnection.Close()
		if err != nil {
			log.Println("WHEPSession.Close.PeerConnection.Error", err)
		}
		log.Println("WHEPSession.Close.PeerConnection.GracefulClose.Completed")

		// Empty tracks
		w.AudioLock.Lock()
		w.VideoLock.Lock()

		w.AudioTrack = nil
		w.VideoTrack = nil

		w.VideoLock.Unlock()
		w.AudioLock.Unlock()

		w.SSESubscribersLock.Lock()
		w.SSESubscribers = make(map[string]sseSubscriber)
		w.SSESubscribersLock.Unlock()

		if w.onClose != nil {
			w.onClose(w.SessionID)
		}
	})
}

func (w *WHEPSession) SetOnClose(onClose func(string)) {
	w.onClose = onClose
}

// Get the current status of the WHEP session
func (w *WHEPSession) GetWHEPSessionStatus() (state SessionState) {
	w.AudioLock.RLock()
	w.VideoLock.Lock()
	w.updateVideoBitrateLocked(time.Now())

	currentAudioLayer := w.AudioLayerCurrent.Load().(string)
	currentVideoLayer := w.VideoLayerCurrent.Load().(string)

	state = SessionState{
		ID: w.SessionID,

		AudioLayerCurrent:   currentAudioLayer,
		AudioTimestamp:      w.AudioTimestamp,
		AudioPacketsWritten: w.AudioPacketsWritten,
		AudioSequenceNumber: uint64(w.AudioSequenceNumber),

		VideoLayerCurrent:   currentVideoLayer,
		VideoTimestamp:      w.VideoTimestamp,
		VideoBitrate:        w.VideoBitrate.Load(),
		VideoPacketsWritten: w.VideoPacketsWritten,
		VideoPacketsDropped: w.VideoPacketsDropped.Load(),
		VideoSequenceNumber: uint64(w.VideoSequenceNumber),
	}

	w.VideoLock.Unlock()
	w.AudioLock.RUnlock()

	return
}

// Finds the corresponding WHIP session to the WHEP session id and sets the requested audio layer
func (w *WHEPSession) SetAudioLayer(encodingID string) {
	log.Println("Setting Audio Layer")
	w.AudioLayerCurrent.Store(encodingID)
	w.IsWaitingForKeyframe.Store(true)
	w.SendPLI()
}

// Finds the corresponding WHIP session to the WHEP session id and sets the requested video layer
func (w *WHEPSession) SetVideoLayer(encodingID string) {
	log.Println("Setting Video Layer")
	w.VideoLayerCurrent.Store(encodingID)
	w.IsWaitingForKeyframe.Store(true)
	w.SendPLI()
}

func (w *WHEPSession) SendPLI() {
	if w.IsSessionClosed.Load() {
		return
	}

	if w.pliSender != nil {
		w.pliSender()
	}
}

func (w *WHEPSession) AddSSESubscriber(subscriberID string, writeEvent func(string) bool, cancel func()) bool {
	w.SSESubscribersLock.Lock()
	defer w.SSESubscribersLock.Unlock()

	if w.IsSessionClosed.Load() || writeEvent == nil || cancel == nil {
		return false
	}

	w.SSESubscribers[subscriberID] = sseSubscriber{
		writeEvent: writeEvent,
		cancel:     cancel,
	}
	return true
}

func (w *WHEPSession) RemoveSSESubscriber(subscriberID string) {
	w.SSESubscribersLock.Lock()
	delete(w.SSESubscribers, subscriberID)
	w.SSESubscribersLock.Unlock()
}

func (w *WHEPSession) BroadcastSSE(message string) {
	if message == "" || w.IsSessionClosed.Load() {
		return
	}

	w.SSESubscribersLock.RLock()
	subscribers := make(map[string]sseSubscriber, len(w.SSESubscribers))
	for id, subscriber := range w.SSESubscribers {
		subscribers[id] = subscriber
	}
	w.SSESubscribersLock.RUnlock()

	for id, subscriber := range subscribers {
		if !subscriber.writeEvent(message) {
			w.RemoveSSESubscriber(id)
			subscriber.cancel()
		}
	}
}

func (w *WHEPSession) updateVideoBitrateLocked(now time.Time) {
	if w.videoBitrateWindowStart.IsZero() {
		w.videoBitrateWindowStart = now
		return
	}

	elapsed := now.Sub(w.videoBitrateWindowStart)
	if elapsed < time.Second {
		return
	}

	bytesDiff := w.VideoBytesWritten - w.videoBitrateWindowBytes
	if bytesDiff < 0 {
		bytesDiff = 0
	}

	w.VideoBitrate.Store(uint64(float64(bytesDiff) / elapsed.Seconds()))
	w.videoBitrateWindowStart = now
	w.videoBitrateWindowBytes = w.VideoBytesWritten
}
