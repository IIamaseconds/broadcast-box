package whip

import (
	"log"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

func (whip *WHIPSession) AddPeerConnection(peerConnection *webrtc.PeerConnection, streamKey string) {
	log.Println("WHIPSession.AddPeerConnection")

	whip.PeerConnectionLock.Lock()
	existingPeerConnection := whip.PeerConnection
	whip.PeerConnection = peerConnection
	whip.PeerConnectionLock.Unlock()

	if existingPeerConnection != nil && existingPeerConnection != peerConnection {
		log.Println("WHIPSession.AddPeerConnection: Replacing existing peerconnection")
		if err := existingPeerConnection.GracefulClose(); err != nil {
			log.Println("WHIPSession.AddPeerConnection.Close.Error", err)
		}
	}

	whip.RegisterWHIPHandlers(peerConnection, streamKey)
}

func (whip *WHIPSession) RemovePeerConnection() {
	log.Println("WHIPSession.RemovePeerConnection", whip.ID)

	whip.PeerConnectionLock.Lock()
	peerConnection := whip.PeerConnection
	whip.PeerConnection = nil
	whip.PeerConnectionLock.Unlock()

	if peerConnection == nil {
		return
	}

	if err := peerConnection.Close(); err != nil {
		log.Println("WHIPSession.RemovePeerConnection.Error", err)
	}

	log.Println("WHIPSession.RemovePeerConnection.Completed", whip.ID)
}

func (whip *WHIPSession) SendPLI() {
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
		log.Println("WHIPSession.SendPLI.WriteRTCP.Error", err)
	}
}

func (whip *WHIPSession) getPLIPackets() []rtcp.Packet {
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
