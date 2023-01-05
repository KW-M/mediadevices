package cmdsource

import (
	"fmt"
	"image"

	"github.com/pion/mediadevices/pkg/io/video"
	"github.com/pion/mediadevices/pkg/prop"
)

type videoCmdTeeTransformer struct {
	cmdSource
}

func getImageBytes(img image.Image) ([]byte, string, string) {
	switch img := img.(type) {
	case *image.YCbCr:
		var bytes []uint8
		bytes = append(bytes, img.Y...)
		bytes = append(bytes, img.Cb...)
		bytes = append(bytes, img.Cr...)
		return bytes, "YCbCr", fmt.Sprintf("%v+", img.SubsampleRatio)
	case *image.CMYK:
		return img.Pix, "CMYK", ""
	case *image.RGBA:
		return img.Pix, "RGBA", ""
	case *image.NRGBA:
		return img.Pix, "NRGBA", ""
	case *image.RGBA64:
		return img.Pix, "RGBA64", ""
	case *image.NRGBA64:
		return img.Pix, "NRGBA64", ""
	case *image.Gray:
		return img.Pix, "Gray", ""
	case *image.Gray16:
		return img.Pix, "Gray16", ""
	case *image.Alpha:
		return img.Pix, "Alpha", ""
	case *image.Alpha16:
		return img.Pix, "Alpha16", ""
	default:
		return nil, "", ""
	}
}

func CreateVideoCmdTeeTransformer(command string, label string, showStdOut bool, showStdErr bool) (video.TransformFunc, error) {
	videoCmdTee := &videoCmdTeeTransformer{
		cmdSource: newCmdSource(command, []prop.Media{}, 0),
	}

	if len(videoCmdTee.cmdArgs) == 0 || videoCmdTee.cmdArgs[0] == "" {
		return nil, errInvalidCommand // no command specified
	}

	err := videoCmdTee.Open()
	if err != nil {
		return nil, err
	}

	writeToVideoTeeFunc, err := videoCmdTee.videoWrite(label, showStdOut, showStdErr)
	if err != nil {
		return nil, err
	}

	envVarsSetFlag := false
	return func(r video.Reader) video.Reader {
		return video.ReaderFunc(func() (image.Image, func(), error) {

			img, _, err := r.Read()
			if err != nil {
				videoCmdTee.Close()
				return nil, func() {}, err
			}
			bytes, pixFormat, pixSubsampleRatio := getImageBytes(img)

			// set the environment variables for the command if they haven't been set yet
			if !envVarsSetFlag {
				envVarsSetFlag = true
				videoCmdTee.addEnvVarsFromStruct(imgMetadata{
					width:             img.Bounds().Dx(),
					height:            img.Bounds().Dy(),
					pixFormat:         pixFormat,
					pixSubsampleRatio: pixSubsampleRatio,
				}, showStdErr)
			}

			// write the image bytes to the command
			err = writeToVideoTeeFunc(bytes)
			if err != nil {
				videoCmdTee.Close()
				return nil, func() {}, err
			}

			return img, func() {}, nil
		})
	}, nil
}

type imgMetadata struct {
	width             int
	height            int
	pixFormat         string
	pixSubsampleRatio string
}

func (c *videoCmdTeeTransformer) videoWrite(label string, showStdOut bool, showStdErr bool) (func([]byte) error, error) {

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
