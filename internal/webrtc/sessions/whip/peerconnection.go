package whip

import (
	"log"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

func (whip *WhipSession) AddPeerConnection(peerConnection *webrtc.PeerConnection, streamKey string) {
	log.Println("WhipSession.AddPeerConnection")

	whip.PeerConnectionLock.Lock()
	existingPeerConnection := whip.PeerConnection
	whip.PeerConnection = peerConnection
	whip.PeerConnectionLock.Unlock()

	if existingPeerConnection != nil && existingPeerConnection != peerConnection {
		log.Println("WhipSession.AddPeerConnection: Replacing existing peerconnection")
		if err := existingPeerConnection.GracefulClose(); err != nil {
			log.Println("WhipSession.AddPeerConnection.Close.Error", err)
		}
	}

	whip.RegisterWhipHandlers(peerConnection, streamKey)
}

func (whip *WhipSession) RemovePeerConnection() {
	log.Println("WhipSession.RemovePeerConnection", whip.Id)

	whip.PeerConnectionLock.Lock()
	peerConnection := whip.PeerConnection
	whip.PeerConnection = nil
	whip.PeerConnectionLock.Unlock()

	if peerConnection == nil {
		return
	}

	if err := peerConnection.Close(); err != nil {
		log.Println("WhipSession.RemovePeerConnection.Error", err)
	}

	log.Println("WhipSession.RemovePeerConnection.Completed", whip.Id)
}

func (whip *WhipSession) SendPLI() {
	whip.PeerConnectionLock.RLock()
	peerConnection := whip.PeerConnection
	whip.PeerConnectionLock.RUnlock()
	if peerConnection == nil {
		return
	}

	packets := whip.getPLIPackets()
	if len(packets) == 0 {
		return
	}

	if err := peerConnection.WriteRTCP(packets); err != nil {
		log.Println("WhipSession.SendPLI.WriteRTCP.Error", err)
	}
}

func (whip *WhipSession) getPLIPackets() []rtcp.Packet {
	whip.TracksLock.RLock()
	defer whip.TracksLock.RUnlock()

	packets := make([]rtcp.Packet, 0, len(whip.VideoTracks))
	for _, track := range whip.VideoTracks {
		if mediaSSRC := track.MediaSSRC.Load(); mediaSSRC != 0 {
			packets = append(packets, &rtcp.PictureLossIndication{
				MediaSSRC: mediaSSRC,
			})
		}
	}

	return packets
}
