package astilibav

import (
	"fmt"

	"github.com/asticode/go-astiav"
)

type FrameAdapter func(f *astiav.Frame) error

func EmptyAudioFrameAdapter(nbSamples, sampleRate int, cl astiav.ChannelLayout, sf astiav.SampleFormat) FrameAdapter {
	return func(f *astiav.Frame) (err error) {
		// Init frame
		f.SetNbSamples(nbSamples)
		f.SetChannelLayout(cl)
		f.SetSampleFormat(sf)
		f.SetSampleRate(sampleRate)

		// Alloc buffer
		// Getting refing frame errors otherwise
		if err = f.AllocBuffer(0); err != nil {
			err = fmt.Errorf("astilibav: allocating buffer failed: %w", err)
			return
		}

		// Alloc samples
		if err = f.AllocSamples(0); err != nil {
			err = fmt.Errorf("astilibav: allocating samples failed: %w", err)
			return
		}
		return
	}
}

func EmptyVideoFrameAdapter(cr astiav.ColorRange, pf astiav.PixelFormat, width, height int) FrameAdapter {
	return func(f *astiav.Frame) (err error) {
		// Init frame
		f.SetColorRange(cr)
		f.SetHeight(height)
		f.SetPixelFormat(pf)
		f.SetWidth(width)

		// Alloc buffer
		// Getting refing frame errors otherwise
		// Aligning with 0 fails
		if err = f.AllocBuffer(1); err != nil {
			err = fmt.Errorf("main: allocating buffer failed: %w", err)
			return
		}

		// Alloc image
		// Aligning with 0 fails
		if err = f.AllocImage(1); err != nil {
			err = fmt.Errorf("main: allocating image failed: %w", err)
			return
		}

		// Image fill black
		if err = f.ImageFillBlack(); err != nil {
			err = fmt.Errorf("main: filling image with black failed: %w", err)
			return
		}
		return
	}
}
