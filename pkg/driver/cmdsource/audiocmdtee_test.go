package cmdsource

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/mediadevices/pkg/wave"
)

func UnsafeCastBytesToInt16s(bytes []byte) []int16 {
	length := len(bytes) / BYTES_IN_INT16
	hdr := reflect.SliceHeader{Data: uintptr(unsafe.Pointer(&bytes[0])), Len: length, Cap: length}
	return *(*[]int16)(unsafe.Pointer(&hdr))
}

func RunAudioCmdTeeTest(t *testing.T, freq int, duration float32, sampleRate int, channelCount int, sampleBufferSize int, format string) {
	sourceCommand := fmt.Sprintf("ffmpeg -hide_banner -f lavfi -i sine=frequency=%d:duration=%f:sample_rate=%d -af arealtime,volume=8 -ac %d -f %s -", freq, duration, sampleRate, channelCount, format)
	sourceTimeout := uint32(10) // 10 seconds
	audioProps := ffmpegAudioFormatMap[format]
	audioProps.ChannelCount = channelCount
	audioProps.SampleRate = sampleRate
	properties := []prop.Media{
		{
			DeviceID: "ffmpeg audio",
			Audio:    audioProps,
		},
	}

	// Make sure ffmpeg is installed before continuting the test
	err := exec.Command("ffmpeg", "-version").Run()
	if err != nil {
		t.Skip("ffmpeg command not found in path. Skipping test. Err: ", err)
	}

	tempOutputFile, err := os.CreateTemp("", "TestImage*.raw")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempOutputFile.Name())

	// ffmpeg command that will save the 0th chunk of the input audio (stdin), and then exit.
	teeCommand := fmt.Sprintf("ffmpeg -hide_banner -f %s -ar %d -ac %d -i pipe:0 -af aselect='lt(samples_n\\,%d)' -f %s -y %s", format, sampleRate, channelCount, sampleBufferSize+2, format, tempOutputFile.Name())
	fmt.Println("Audio source cmd: " + sourceCommand)
	fmt.Println("Audio tee cmd: " + teeCommand)

	// Create the audio cmd source
	audioCmdSource := &audioCmdSource{
		cmdSource:         newCmdSource(sourceCommand, properties, sourceTimeout),
		bufferSampleCount: sampleBufferSize,
		label:             "ffmpeg audio",
		showStdErr:        true,
	}

	err = audioCmdSource.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer audioCmdSource.Close()

	// Get the audio sourceReader from the audio cmd source
	sourceReader, err := audioCmdSource.AudioRecord(properties[0])
	if err != nil {
		t.Fatal(err)
	}

	transform, err := CreateAudioCmdTeeTransformer(teeCommand, "test_audio_tee", true, true)
	if err != nil {
		t.Fatal(err)
	}

	// Read the first chunk
	reader := transform(sourceReader)
	chunk, _, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}

	// Check if the chunk has the correct number of channels
	if chunk.ChunkInfo().Channels != channelCount {
		t.Errorf("chunk has incorrect number of channels")
	}

	// Check if the chunk has the correct sample rate
	if chunk.ChunkInfo().SamplingRate != sampleRate {
		t.Errorf("chunk has incorrect sample rate")
	}

	// read the next 10 chunks for fun
	for i := 0; i < 10; i++ {
		_, _, err := reader.Read()
		if err != nil {
			break
		}
	}

	// Wait to really make sure the tee command has processed the first chunk.
	<-time.After(1 * time.Second)

	teeChunkBytes, err := os.ReadFile(tempOutputFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	print("Tee chunk bytes: " + fmt.Sprintf("%d", len(teeChunkBytes)))
	teeChunk := wave.NewInt16Interleaved(chunk.ChunkInfo())
	teeChunk.Data = UnsafeCastBytesToInt16s(teeChunkBytes)

	// Test the waveform value at the 1st sample in the chunk (should be "near" 0, because it is a sine wave)
	sampleIdx := 1
	channelIdx := 0
	min := int64(0)
	max := int64(267911168)
	value := chunk.At(sampleIdx, channelIdx).Int()
	teeValue := teeChunk.At(sampleIdx, channelIdx).Int()
	if teeValue != value {
		t.Errorf("chan #%d, chunk #%d has incorrect value, expected %d-%d, got %d", channelIdx, sampleIdx, min, max, value)
	}

	// // Test the waveform value at the 1/4th the way through the sine wave (should be near max in 32 bit int)
	// samplesPerSinewaveCycle := sampleRate / freq
	// sampleIdx = samplesPerSinewaveCycle / 4 // 1/4 of a cycle
	// channelIdx = 0
	// min = int64(maxInt32) - int64(267911168)
	// max = 0xFFFFFFFF
	// if value := chunk.At(sampleIdx, channelIdx).Int(); ValueInRange(value, min, max) == false {
	// 	t.Errorf("chan #%d, chunk #%d has incorrect value, expected %d-%d, got %d", channelIdx, sampleIdx, min, max, value)
	// }

	err = audioCmdSource.Close()
	if err != nil && err.Error() != "exit status 255" { // ffmpeg returns 255 when it is stopped normally
		t.Fatal(err)
	}

	audioCmdSource.Close() // should not panic
}

func TestWavIntLeAudioCmdTeeOut(t *testing.T) {
	RunAudioCmdTeeTest(t, 440, 1, 44100, 1, 256, "s16le")
}

// func TestWavIntBeAudioCmdOut(t *testing.T) {
// 	RunAudioCmdTest(t, 120, 1, 44101, 1, 256, "s16be")
// }

// func TestWavFloatLeAudioCmdOut(t *testing.T) {
// 	RunAudioCmdTest(t, 220, 1, 44102, 1, 256, "f32le")
// }

// func TestWavFloatBeAudioCmdOut(t *testing.T) {
// 	RunAudioCmdTest(t, 110, 1, 44103, 1, 256, "f32be")
// }
