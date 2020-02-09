package mediadevices

import (
	"fmt"
	"io"
	"math/rand"

	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/mediadevices/pkg/driver"
	mio "github.com/pion/mediadevices/pkg/io"
	"github.com/pion/webrtc/v2"
	"github.com/pion/webrtc/v2/pkg/media"
)

// Tracker is an interface that represent MediaStreamTrack
// Reference: https://w3c.github.io/mediacapture-main/#mediastreamtrack
type Tracker interface {
	Track() *webrtc.Track
	Stop()
}

type track struct {
	t *webrtc.Track
	s *sampler
}

func newTrack(codecs []*webrtc.RTPCodec, d driver.Driver, codecName string) (*track, error) {
	var selectedCodec *webrtc.RTPCodec
	for _, c := range codecs {
		if c.Name == codecName {
			selectedCodec = c
			break
		}
	}
	if selectedCodec == nil {
		return nil, fmt.Errorf("track: %s is not registered in media engine", codecName)
	}

	t, err := webrtc.NewTrack(
		selectedCodec.PayloadType,
		rand.Uint32(),
		selectedCodec.Type.String(),
		d.ID(),
		selectedCodec,
	)
	if err != nil {
		return nil, err
	}

	return &track{
		t: t,
		s: newSampler(t),
	}, nil
}

func (t *track) Track() *webrtc.Track {
	return t.t
}

type videoTrack struct {
	*track
	d           driver.Driver
	constraints MediaTrackConstraints
	encoder     io.ReadCloser
}

var _ Tracker = &videoTrack{}

func newVideoTrack(codecs []*webrtc.RTPCodec, d driver.Driver, constraints MediaTrackConstraints) (*videoTrack, error) {
	codecName := constraints.Codec
	t, err := newTrack(codecs, d, codecName)
	if err != nil {
		return nil, err
	}

	err = d.Open()
	if err != nil {
		return nil, err
	}

	vr := d.(driver.VideoRecorder)
	r, err := vr.VideoRecord(constraints.Media)
	if err != nil {
		return nil, err
	}

	// TODO: Remove hardcoded bitrate
	constraints.BitRate = 100000
	encoder, err := codec.BuildVideoEncoder(codecName, r, constraints.Video)
	if err != nil {
		return nil, err
	}

	vt := videoTrack{
		track:       t,
		d:           d,
		constraints: constraints,
		encoder:     encoder,
	}

	go vt.start()
	return &vt, nil
}

func (vt *videoTrack) start() {
	var n int
	var err error
	buff := make([]byte, 1024)
	for {
		n, err = vt.encoder.Read(buff)
		if err != nil {
			if e, ok := err.(*mio.InsufficientBufferError); ok {
				buff = make([]byte, 2*e.RequiredSize)
				continue
			}

			// TODO: better error handling
			panic(err)
		}

		vt.s.sample(buff[:n])
	}
}

func (vt *videoTrack) Stop() {
	vt.d.Close()
	vt.encoder.Close()
}

type audioTrack struct {
	*track
	d           driver.Driver
	constraints MediaTrackConstraints
	encoder     io.ReadCloser
}

var _ Tracker = &audioTrack{}

func newAudioTrack(codecs []*webrtc.RTPCodec, d driver.Driver, constraints MediaTrackConstraints) (*audioTrack, error) {
	codecName := constraints.Codec
	t, err := newTrack(codecs, d, codecName)
	if err != nil {
		return nil, err
	}

	err = d.Open()
	if err != nil {
		return nil, err
	}

	ar := d.(driver.AudioRecorder)
	reader, err := ar.AudioRecord(constraints.Media)
	if err != nil {
		return nil, err
	}

	encoder, err := codec.BuildAudioEncoder(codecName, reader, constraints.Audio)
	if err != nil {
		return nil, err
	}

	at := audioTrack{
		track:       t,
		d:           d,
		constraints: constraints,
		encoder:     encoder,
	}
	go at.start()
	return &at, nil
}

func (t *audioTrack) start() {
	buff := make([]byte, 1024)
	sampleSize := uint32(float64(t.constraints.SampleRate) * t.constraints.Latency.Seconds())
	for {
		n, err := t.encoder.Read(buff)
		if err != nil {
			// TODO: better error handling
			panic(err)
		}

		t.t.WriteSample(media.Sample{
			Data:    buff[:n],
			Samples: sampleSize,
		})
	}
}

func (t *audioTrack) Stop() {
	t.d.Close()
	t.encoder.Close()
}