package whip

import (
	"log"
	"time"

	"github.com/glimesh/broadcast-box/internal/webrtc/codecs"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

// Add a new AudioTrack to the WHIP session
func (w *WHIPSession) AddAudioTrack(rid string, streamKey string, codec codecs.TrackCodeType) (*AudioTrack, error) {
	log.Println("WHIPSession.AddAudioTrack:", streamKey, "(", rid, ")")
	w.TracksLock.Lock()
	defer w.TracksLock.Unlock()

	if existingTrack, ok := w.AudioTracks[rid]; ok {
		return existingTrack, nil
	}

	track := &AudioTrack{
		Rid: rid,
		Track: codecs.CreateTrackMultiCodec(
			"audio-"+uuid.New().String(),
			rid,
			streamKey,
			webrtc.RTPCodecTypeAudio,
			codec),
	}
	track.LastReceived.Store(time.Time{})

	w.AudioTracks[track.Rid] = track

	return track, nil
}

// Add a new VideoTrack to the WHIP session
func (w *WHIPSession) AddVideoTrack(rid string, streamKey string, codec codecs.TrackCodeType) (*VideoTrack, error) {
	log.Println("WHIPSession.AddVideoTrack:", "(", rid, ")")
	w.TracksLock.Lock()
	defer w.TracksLock.Unlock()

	if existingTrack, ok := w.VideoTracks[rid]; ok {
		return existingTrack, nil
	}

	track := &VideoTrack{
		Rid: rid,
		Track: codecs.CreateTrackMultiCodec(
			"video-"+uuid.New().String(),
			rid,
			streamKey,
			webrtc.RTPCodecTypeVideo,
			codec),
	}
	track.LastReceived.Store(time.Time{})

	w.VideoTracks[rid] = track

	return track, nil
}

// Remove Audio and Video tracks coming from the whip session id
func (w *WHIPSession) RemoveTracks() {
	log.Println("WHIPSession.RemoveTracks")

	w.TracksLock.Lock()
	w.AudioTracks = make(map[string]*AudioTrack)
	w.VideoTracks = make(map[string]*VideoTrack)
	w.TracksLock.Unlock()
}

// Get highest prioritized audio track in the whip session
// This only works if the priority has been set.
// Currently this is only supported when being set through the simulcast
// property in the offer made by the whip connection
func (w *WHIPSession) GetHighestPrioritizedAudioTrack() string {
	if len(w.AudioTracks) == 0 {
		log.Println("No Audio tracks was found for", w.ID)
		return ""
	}

	w.TracksLock.RLock()
	var highestPriorityAudioTrack *AudioTrack
	for _, trackPriority := range w.AudioTracks {
		if highestPriorityAudioTrack == nil {
			highestPriorityAudioTrack = trackPriority
			continue
		}

		if trackPriority.Priority < highestPriorityAudioTrack.Priority {
			highestPriorityAudioTrack = trackPriority
		}
	}
	w.TracksLock.RUnlock()

	if highestPriorityAudioTrack == nil {
		return ""
	}

	return highestPriorityAudioTrack.Rid

}

// Get highest prioritized video track in the whip session
// This only works if the priority has been set.
// Currently this is only supported when being set through the simulcast
// property in the offer made by the whip connection
func (w *WHIPSession) GetHighestPrioritizedVideoTrack() string {
	if len(w.VideoTracks) == 0 {
		log.Println("No Video tracks was found for", w.ID)
	}

	var highestPriorityVideoTrack *VideoTrack

	w.TracksLock.RLock()
	for _, trackPriority := range w.VideoTracks {
		if highestPriorityVideoTrack == nil {
			highestPriorityVideoTrack = trackPriority
			continue
		}

		if trackPriority.Priority < highestPriorityVideoTrack.Priority {
			highestPriorityVideoTrack = trackPriority
		}
	}
	w.TracksLock.RUnlock()

	if highestPriorityVideoTrack == nil {
		return ""
	}

	return highestPriorityVideoTrack.Rid
}
