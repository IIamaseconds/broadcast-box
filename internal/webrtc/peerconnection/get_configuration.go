package peerconnection

import (
	"os"
	"strings"

	"github.com/glimesh/broadcast-box/internal/environment"
	"github.com/glimesh/broadcast-box/internal/server/authorization"
	"github.com/pion/webrtc/v4"
)

func GetPeerConnectionConfig() webrtc.Configuration {
	config := webrtc.Configuration{}
	if stunServers := os.Getenv(environment.STUNServersInternal); stunServers != "" {
		for stunServer := range strings.SplitSeq(stunServers, "|") {
			config.ICEServers = append(config.ICEServers, webrtc.ICEServer{
				URLs: []string{"stun:" + stunServer},
			})
		}
	} else if stunServers := os.Getenv(environment.STUNServers); stunServers != "" {
		for stunServer := range strings.SplitSeq(stunServers, "|") {
			config.ICEServers = append(config.ICEServers, webrtc.ICEServer{
				URLs: []string{"stun:" + stunServer},
			})
		}
	}

	username, credential := authorization.GetTURNCredentials()

	if turnServers := os.Getenv(environment.TURNServersInternal); turnServers != "" {
		for turnServer := range strings.SplitSeq(turnServers, "|") {
			config.ICEServers = append(config.ICEServers, webrtc.ICEServer{
				URLs:       []string{"turn:" + turnServer},
				Username:   username,
				Credential: credential,
			})
		}
	} else if turnServers := os.Getenv(environment.TURNServers); turnServers != "" {
		for turnServer := range strings.SplitSeq(turnServers, "|") {
			config.ICEServers = append(config.ICEServers, webrtc.ICEServer{
				URLs:       []string{"turn:" + turnServer},
				Username:   username,
				Credential: credential,
			})
		}
	}

	return config
}
