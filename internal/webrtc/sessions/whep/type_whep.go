package whep

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/glimesh/broadcast-box/internal/webrtc/codecs"
	"github.com/pion/webrtc/v4"
)

type (
	sseSubscriber struct {
		writeEvent func(string) bool
		cancel     func()
	}

	WhepSession struct {
		SessionId            string
		IsWaitingForKeyframe atomic.Bool
		IsSessionClosed      atomic.Bool

		SseSubscribersLock  sync.RWMutex
		SseSubscribers      map[string]sseSubscriber
		SessionClose        sync.Once
		ActiveContext       context.Context
		ActiveContextCancel func()
		pliSender           func()

		PeerConnectionLock sync.RWMutex
		PeerConnection     *webrtc.PeerConnection

		// Protects VideoTrack, VideoTimestamp, VideoPacketsWritten, VideoSequenceNumber
		VideoLock               sync.RWMutex
		VideoTrack              *codecs.TrackMultiCodec
		VideoTimestamp          uint32
		VideoBitrate            atomic.Uint64
		VideoBytesWritten       int
		videoBitrateWindowStart time.Time
		videoBitrateWindowBytes int
		VideoPacketsWritten     uint64
		VideoPacketsDropped     atomic.Uint64
		VideoSequenceNumber     uint16
		VideoLayerCurrent       atomic.Value

		// Protects AudioTrack, AudioTimestamp, AudioPacketsWritten, AudioSequenceNumber
		AudioLock           sync.RWMutex
		AudioTrack          *codecs.TrackMultiCodec
		AudioTimestamp      uint32
		AudioPacketsWritten uint64
		AudioSequenceNumber uint16
		AudioLayerCurrent   atomic.Value
	}
)
