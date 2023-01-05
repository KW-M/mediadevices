package cmdsource

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/pion/mediadevices/pkg/io/audio"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/mediadevices/pkg/wave"
)

const BYTES_IN_INT16 = 2
const BYTES_IN_FLOAT32 = 4

func UnsafeCastInt16sToBytes(ints []int16) []byte {
	length := len(ints) * BYTES_IN_INT16
	hdr := reflect.SliceHeader{Data: uintptr(unsafe.Pointer(&ints[0])), Len: length, Cap: length}
	return *(*[]byte)(unsafe.Pointer(&hdr))
}

func UnsafeCastFloat32sToBytes(floats []float32) []byte {
	length := len(floats) * BYTES_IN_FLOAT32
	hdr := reflect.SliceHeader{Data: uintptr(unsafe.Pointer(&floats[0])), Len: length, Cap: length}
	return *(*[]byte)(unsafe.Pointer(&hdr))
}

type audioCmdTeeTransformer struct {
	cmdSource
}

func getChunkBytes(chunk wave.Audio) ([]byte, string) {
	switch chunk := chunk.(type) {
	case *wave.Float32Interleaved:
		data := UnsafeCastFloat32sToBytes(chunk.Data)
		return data, "Float32Interleaved"
	case *wave.Int16Interleaved:
		data := UnsafeCastInt16sToBytes(chunk.Data)
		return data, "Int16Interleaved"
	case *wave.Float32NonInterleaved:
		var data []byte
		for _, channel := range chunk.Data {
			data = append(data, UnsafeCastFloat32sToBytes(channel)...)
		}
		return data, "Float32NonInterleaved"
	case *wave.Int16NonInterleaved:
		var data []byte
		for _, channel := range chunk.Data {
			data = append(data, UnsafeCastInt16sToBytes(channel)...)
		}
		return data, "Int16NonInterleaved"
	default:
		return nil, ""
	}
}

func CreateAudioCmdTeeTransformer(command string, label string, showStdOut bool, showStdErr bool) (audio.TransformFunc, error) {
	audioCmdTee := &audioCmdTeeTransformer{
		cmdSource: newCmdSource(command, []prop.Media{}, 0),
	}

	if len(audioCmdTee.cmdArgs) == 0 || audioCmdTee.cmdArgs[0] == "" {
		return nil, errInvalidCommand // no command specified
	}

	err := audioCmdTee.Open()
	if err != nil {
		return nil, err
	}

	writeToaudioTeeFunc, err := audioCmdTee.audioWrite(label, showStdOut, showStdErr)
	if err != nil {
		return nil, err
	}

	envVarsSetFlag := false
	return func(r audio.Reader) audio.Reader {
		return audio.ReaderFunc(func() (wave.Audio, func(), error) {
			chunk, _, err := r.Read()
			if err != nil {
				audioCmdTee.Close()
				return nil, func() {}, err
			}
			bytes, _ := getChunkBytes(chunk)

			// set the environment variables for the command if they haven't been set yet
			if !envVarsSetFlag {
				envVarsSetFlag = true
				audioCmdTee.addEnvVarsFromStruct(chunk.ChunkInfo(), showStdErr)
			}

			// write the image bytes to the command
			err = writeToaudioTeeFunc(bytes)
			if err != nil {
				audioCmdTee.Close()
				return nil, func() {}, err
			}

			return chunk, func() {}, nil
		})
	}, nil
}

func (c *audioCmdTeeTransformer) audioWrite(label string, showStdOut bool, showStdErr bool) (func([]byte) error, error) {

	if showStdOut {
		// get the command's standard error
		stdOut, err := c.execCmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		// send standard error to the console as debug logs prefixed with "{command} stdOut >"
		go c.logStdIoWithPrefix(fmt.Sprintf("%s stdOut> ", label+":"+c.cmdArgs[0]), stdOut)
	}

	if showStdErr {
		// get the command's standard error
		stdErr, err := c.execCmd.StderrPipe()
		if err != nil {
			return nil, err
		}
		// send standard error to the console as debug logs prefixed with "{command} stdErr >"
		go c.logStdIoWithPrefix(fmt.Sprintf("%s stdErr> ", label+":"+c.cmdArgs[0]), stdErr)
	}

	stdIn, err := c.execCmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	// start the command
	if err := c.execCmd.Start(); err != nil {
		return nil, err
	}

	// return a function that writes to the command's standard input
	return func(inputBytes []byte) error {
		_, err := stdIn.Write(inputBytes)
		return err
	}, nil
}
