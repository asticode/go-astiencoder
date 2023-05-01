package astilibav

import (
	"sync"
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestFilterer(t *testing.T) {
	eh := astiencoder.NewEventHandler()
	eh.AddForEventName(astiencoder.EventNameError, func(e astiencoder.Event) (deleteListener bool) {
		t.Log(e.Payload)
		return
	})
	c := astikit.NewCloser()
	defer c.Close()
	s := astiencoder.NewStater(time.Second, eh)
	wk := astikit.NewWorker(astikit.WorkerOptions{})
	defer wk.Stop()

	n1 := NewForwarder(ForwarderOptions{OutputCtx: Context{
		MediaType:         astiav.MediaTypeVideo,
		Height:            2,
		SampleAspectRatio: astiav.NewRational(1, 1),
		TimeBase:          astiav.NewRational(1, 30),
		Width:             4,
	}}, eh, c, s)
	n2 := NewForwarder(ForwarderOptions{OutputCtx: Context{
		MediaType:         astiav.MediaTypeVideo,
		Height:            6,
		SampleAspectRatio: astiav.NewRational(1, 1),
		TimeBase:          astiav.NewRational(1, 30),
		Width:             12,
	}}, eh, c, s)

	fm1 := astiav.AllocFrame()
	defer fm1.Free()
	require.NoError(t, EmptyVideoFrameAdapter(astiav.ColorRangeUnspecified, astiav.PixelFormatYuv420P, 4, 2)(fm1))
	fm1.SetSampleAspectRatio(astiav.NewRational(1, 1))
	fm2 := astiav.AllocFrame()
	defer fm2.Free()
	require.NoError(t, EmptyVideoFrameAdapter(astiav.ColorRangeUnspecified, astiav.PixelFormatYuv420P, 8, 4)(fm2))
	fm2.SetSampleAspectRatio(astiav.NewRational(1, 1))
	fm3 := astiav.AllocFrame()
	defer fm3.Free()
	require.NoError(t, EmptyVideoFrameAdapter(astiav.ColorRangeUnspecified, astiav.PixelFormatYuv420P, 12, 6)(fm3))
	fm3.SetSampleAspectRatio(astiav.NewRational(1, 1))

	fc := "[1]setpts=N[p1];[2]setpts=N[p2];[p2][p1]overlay"
	f1, err := NewFilterer(FiltererOptions{
		Content:              fc,
		FrameHandlerStrategy: FiltererFrameHandlerStrategyDefault,
		Inputs: map[string]astiencoder.Node{
			"1": n1,
			"2": n2,
		},
		Node: astiencoder.NodeOptions{Metadata: astiencoder.NodeMetadata{Name: "f1"}},
		OutputCtx: Context{
			MediaType: astiav.MediaTypeVideo,
			TimeBase:  astiav.NewRational(1, 30),
		},
	}, eh, c, s)
	require.NoError(t, err)
	var inputContextChanges []string
	f2, err := NewFilterer(FiltererOptions{
		Content:              fc,
		FrameHandlerStrategy: FiltererFrameHandlerStrategyPTS,
		Inputs: map[string]astiencoder.Node{
			"1": n1,
			"2": n2,
		},
		Node: astiencoder.NodeOptions{Metadata: astiencoder.NodeMetadata{Name: "f2"}},
		OnInputContextChanges: func(cs FiltererInputContextChanges, f *Filterer) (ignore bool) {
			inputContextChanges = append(inputContextChanges, cs.String())
			return
		},
		OutputCtx: Context{
			MediaType: astiav.MediaTypeVideo,
			TimeBase:  astiav.NewRational(1, 30),
		},
	}, eh, c, s)
	require.NoError(t, err)

	type interceptedFrame struct {
		h   int
		pts int64
		w   int
	}
	m := &sync.Mutex{}
	interceptedFrames := make(map[string][]interceptedFrame)
	i := newFrameInterceptor(func(p FrameHandlerPayload) {
		m.Lock()
		defer m.Unlock()
		interceptedFrames[p.Node.Metadata().Name] = append(interceptedFrames[p.Node.Metadata().Name], interceptedFrame{
			h:   p.Frame.Height(),
			pts: p.Frame.Pts(),
			w:   p.Frame.Width(),
		})
		if len(interceptedFrames["f1"]) == 3 && len(interceptedFrames["f2"]) == 3 {
			wk.Stop()
		}
	}, eh, c, s)
	f1.Connect(i)
	f2.Connect(i)

	dispatch := func(f *astiav.Frame, pts int64, n astiencoder.Node) {
		f.SetPts(pts)
		f1.HandleFrame(FrameHandlerPayload{Frame: f, Node: n})
		f2.HandleFrame(FrameHandlerPayload{Frame: f, Node: n})
	}

	dispatch(fm1, 0, n1)
	dispatch(fm1, 1, n1)
	dispatch(fm3, 1, n2)
	dispatch(fm1, 2, n1)
	dispatch(fm1, 3, n1)
	dispatch(fm3, 3, n2)
	dispatch(fm1, 4, n1)
	dispatch(fm2, 4, n2)

	f1.Start(wk.Context(), wk.NewTask)
	f2.Start(wk.Context(), wk.NewTask)

	wk.Wait()

	require.Equal(t, map[string][]interceptedFrame{
		"f1": {
			{
				h:   6,
				pts: 0,
				w:   12,
			},
			{
				h:   6,
				pts: 1,
				w:   12,
			},
			{
				h:   4,
				pts: 0,
				w:   8,
			},
		},
		"f2": {
			{
				h:   6,
				pts: 1,
				w:   12,
			},
			{
				h:   6,
				pts: 3,
				w:   12,
			},
			{
				h:   4,
				pts: 4,
				w:   8,
			},
		},
	}, interceptedFrames)
	require.Equal(t, []string{"forwarder_2: height changed: 6 --> 4 && width changed: 12 --> 8"}, inputContextChanges)
	require.Len(t, f1.items, 2)
	require.Len(t, f2.items, 0)
}
