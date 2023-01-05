package cmdsource

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
)

func RunVideoTeeCmdTest(t *testing.T, width int, height int, frameRate float32, frameFormat frame.Format, inputColor string, expectedColor color.Color) {
	sourceCommand := fmt.Sprintf("ffmpeg -hide_banner -f lavfi -i color=c=%s:size=%dx%d:rate=%f -vf realtime -f rawvideo -pix_fmt %s -", inputColor, width, height, frameRate, ffmpegFrameFormatMap[frameFormat])
	sourceTimeout := uint32(10) // 10 seconds
	sourceProperties := []prop.Media{
		{
			DeviceID: "ffmpeg 1",
			Video: prop.Video{
				Width:       width,
				Height:      height,
				FrameFormat: frameFormat,
				FrameRate:   frameRate,
			},
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

	// ffmpeg command that will save the 0th frame of the input video (stdin), and then exit.
	teeCommand := fmt.Sprintf("ffmpeg -hide_banner -f rawvideo -pix_fmt %s -s %dx%d -i pipe:0 -vf select='eq(n\\,0)' -vframes 1 -f rawvideo -pix_fmt %s -y %s", ffmpegFrameFormatMap[frameFormat], width, height, ffmpegFrameFormatMap[frameFormat], tempOutputFile.Name())
	fmt.Println("Video source cmd: " + sourceCommand)
	fmt.Println("Video tee cmd: " + teeCommand)

	// Create a new video command source
	videoCmdSource := &videoCmdSource{
		cmdSource: newCmdSource(sourceCommand, sourceProperties, sourceTimeout),
	}

	err = videoCmdSource.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer videoCmdSource.Close()

	sourceReader, err := videoCmdSource.VideoRecord(sourceProperties[0])
	if err != nil {
		t.Fatal(err)
	}

	transform, err := CreateVideoCmdTeeTransformer(teeCommand, "test_tee", true, true)
	if err != nil {
		t.Fatal(err)
	}

	reader := transform(sourceReader)
	img, _, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}

	// read ten more frames for fun
	for i := 0; i < 10; i++ {
		_, _, err := reader.Read()
		if err != nil {
			break
		}
	}

	// Wait to really make sure the tee command has processed the first frame.
	<-time.After(1 * time.Second)

	teeImgBytes, err := os.ReadFile(tempOutputFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	teeImg := image.NewYCbCr(image.Rect(0, 0, width, height), image.YCbCrSubsampleRatio420)
	teeImg.Y = teeImgBytes[0 : width*height]
	teeImg.Cb = teeImgBytes[width*height : width*height*5/4]
	teeImg.Cr = teeImgBytes[width*height*5/4 : width*height*3/2]

	// test color at upper left corner
	pxlColor := img.At(0, 0)
	teePxlColor := teeImg.At(0, 0)
	if pxlColor != teePxlColor {
		t.Errorf("Image pixel output at 0,0 doesn't match tee. Got: %+v | Expected: %+v", pxlColor, teePxlColor)
	}

	// test color at center of image
	x := width / 2
	y := height / 2
	pxlColor = img.At(x, y)
	teePxlColor = teeImg.At(x, y)
	if pxlColor != teePxlColor {
		t.Errorf("Image pixel output at %d,%d doesn't match tee. Got: %+v | Expected: %+v", x, y, pxlColor, teePxlColor)
	}

	// test color at lower right corner
	x = width - 1
	y = height - 1
	pxlColor = img.At(x, y)
	teePxlColor = teeImg.At(x, y)
	if pxlColor != teePxlColor {
		t.Errorf("Image pixel output at %d,%d is not correct. Got: %+v | Expected: %+v", x, y, pxlColor, teePxlColor)
	}

	err = videoCmdSource.Close()
	if err != nil {
		t.Fatal(err)
	}
	videoCmdSource.Close() // should not panic
	println()              // add a new line to separate the output from the end of the test
}

func TestI420VideoTeeCmdOut(t *testing.T) {
	RunVideoTeeCmdTest(t, 640, 480, 30, frame.FormatI420, "pink", ycbcrPink)
}
