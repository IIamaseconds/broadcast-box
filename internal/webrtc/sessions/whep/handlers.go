package whep

import (
	"log"

	"github.com/pion/webrtc/v4"
)

func (w *WHEPSession) RegisterWHEPHandlers(peerConnection *webrtc.PeerConnection) {
	log.Println("WHEPSession.RegisterHandlers")

	peerConnection.OnICEConnectionStateChange(onWHEPICEConnectionStateChangeHandler(w))
}

func onWHEPICEConnectionStateChangeHandler(w *WHEPSession) func(webrtc.ICEConnectionState) {
	return func(state webrtc.ICEConnectionState) {
		log.Println("WHEPSession.OnICEConnectionStateChange:", state)
		switch state {
		case
			webrtc.ICEConnectionStateConnected:
			w.SendPLI()
		case
			webrtc.ICEConnectionStateFailed,
			webrtc.ICEConnectionStateClosed:
			w.Close()
		default:
			log.Println("WHEPSession.OnICEConnectionStateChange.Default", state)
		}
	}
}
