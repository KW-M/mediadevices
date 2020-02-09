package driver

import (
	"github.com/pion/mediadevices/pkg/io/audio"
	"github.com/pion/mediadevices/pkg/io/video"
	"github.com/pion/mediadevices/pkg/prop"
	uuid "github.com/satori/go.uuid"
)

func wrapAdapter(a Adapter) Driver {
	id := uuid.NewV4().String()
	d := &adapterWrapper{Adapter: a, id: id, state: StateClosed}

	switch v := a.(type) {
	case VideoRecorder:
		// Only expose Driver and VideoRecorder interfaces
		d.VideoRecorder = v
		r := &struct {
			Driver
			VideoRecorder
		}{d, d}
		return r
	case AudioRecorder:
		// Only expose Driver and AudioRecorder interfaces
		d.AudioRecorder = v
		return &struct {
			Driver
			AudioRecorder
		}{d, d}
	default:
		panic("adapter has to be either VideoRecorder/AudioRecorder")
	}
}

type adapterWrapper struct {
	Adapter
	VideoRecorder
	AudioRecorder
	id    string
	state State
}

func (w *adapterWrapper) ID() string {
	return w.id
}

func (w *adapterWrapper) Status() State {
	return w.state
}

func (w *adapterWrapper) Open() error {
	return w.state.Update(StateOpened, w.Adapter.Open)
}

func (w *adapterWrapper) Close() error {
	return w.state.Update(StateClosed, w.Adapter.Close)
}

func (w *adapterWrapper) VideoRecord(p prop.Media) (r video.Reader, err error) {
	w.state.Update(StateRunning, func() error {
		r, err = w.VideoRecorder.VideoRecord(p)
		return err
	})
	return
}

func (w *adapterWrapper) AudioRecord(p prop.Media) (r audio.Reader, err error) {
	w.state.Update(StateRunning, func() error {
		r, err = w.AudioRecorder.AudioRecord(p)
		return err
	})
	return
}