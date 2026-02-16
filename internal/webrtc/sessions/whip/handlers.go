package whip

import (
	"log"
	"strings"

	"github.com/pion/webrtc/v4"
)

func (whip *WHIPSession) RegisterWHIPHandlers(peerConnection *webrtc.PeerConnection, streamKey string) {
	log.Println("WHIPSession.RegisterHandlers")

	// PeerConnection OnTrack handler
	whip.PeerConnection.OnTrack(whip.onTrackHandler(peerConnection, streamKey))

	// PeerConnection OnICEConnectionStateChange handler
	whip.PeerConnection.OnICEConnectionStateChange(whip.onICEConnectionStateChangeHandler())

	// PeerConnection OnConnectionStateChange
	whip.PeerConnection.OnConnectionStateChange(whip.onConnectionStateChange())
}

func (whip *WHIPSession) onICEConnectionStateChangeHandler() func(webrtc.ICEConnectionState) {
	return func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateFailed || state == webrtc.ICEConnectionStateClosed {
			log.Println("WHIPSession.PeerConnection.OnICEConnectionStateChange", whip.ID)
			whip.ActiveContextCancel()
		}
	}
}

func (whip *WHIPSession) onTrackHandler(peerConnection *webrtc.PeerConnection, streamKey string) func(*webrtc.TrackRemote, *webrtc.RTPReceiver) {
	return func(remoteTrack *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {
		log.Println("WHIPSession.PeerConnection.OnTrackHandler", whip.ID)

		if strings.HasPrefix(remoteTrack.Codec().MimeType, "audio") {
			// Handle audio stream
			whip.AudioWriter(remoteTrack, streamKey, peerConnection)
		} else {
			// Handle video stream
			whip.VideoWriter(remoteTrack, streamKey, peerConnection)
		}

		log.Println("WHIPSession.OnTrackHandler.TrackStopped", remoteTrack.RID())
	}
}

func (whip *WHIPSession) onConnectionStateChange() func(webrtc.PeerConnectionState) {
	return func(state webrtc.PeerConnectionState) {
		log.Println("WHIPSession.PeerConnection.OnConnectionStateChange", state)

		switch state {
		case webrtc.PeerConnectionStateClosed:
		case webrtc.PeerConnectionStateFailed:
			log.Println("WHIPSession.PeerConnection.OnConnectionStateChange: Host removed", whip.ID)
			whip.ActiveContextCancel()

		case webrtc.PeerConnectionStateConnected:
			log.Println("WHIPSession.PeerConnection.OnConnectionStateChange: Host connected", whip.ID)

		}
	}
}
