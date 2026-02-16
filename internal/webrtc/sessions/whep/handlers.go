package whep

import (
	"log"

	"github.com/pion/webrtc/v4"
)

func (whep *WHEPSession) RegisterWHEPHandlers(peerConnection *webrtc.PeerConnection) {
	log.Println("WHEPSession.RegisterHandlers")

	peerConnection.OnICEConnectionStateChange(onWHEPICEConnectionStateChangeHandler(whep))
}

func onWHEPICEConnectionStateChangeHandler(whep *WHEPSession) func(webrtc.ICEConnectionState) {
	return func(state webrtc.ICEConnectionState) {
		log.Println("WHEPSession.OnICEConnectionStateChange:", state)
		switch state {
		case
			webrtc.ICEConnectionStateConnected:
			whep.SendPLI()
		case
			webrtc.ICEConnectionStateFailed,
			webrtc.ICEConnectionStateClosed:
			whep.Close()
		default:
			log.Println("WHEPSession.OnICEConnectionStateChange.Default", state)
		}
	}
}
