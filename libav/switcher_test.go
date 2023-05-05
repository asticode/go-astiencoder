package astilibav

import (
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestSwitcher(t *testing.T) {
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

	n1 := NewForwarder(ForwarderOptions{Node: astiencoder.NodeOptions{Metadata: astiencoder.NodeMetadata{Name: "n1"}}}, eh, c, s)
	n2 := NewForwarder(ForwarderOptions{Node: astiencoder.NodeOptions{Metadata: astiencoder.NodeMetadata{Name: "n2"}}}, eh, c, s)

	f1 := astiav.AllocFrame()
	defer f1.Free()
	require.NoError(t, EmptyVideoFrameAdapter(astiav.ColorRangeUnspecified, astiav.PixelFormatYuv420P, 1, 1)(f1))
	f2 := astiav.AllocFrame()
	defer f2.Free()
	require.NoError(t, EmptyVideoFrameAdapter(astiav.ColorRangeUnspecified, astiav.PixelFormatYuv420P, 2, 2)(f2))

	sw, err := NewSwitcher(SwitcherOptions{
		FrameDuration: 1,
		FrameTimeout:  10 * time.Millisecond,
	}, eh, c, s)
	require.NoError(t, err)

	dispatch := func(n astiencoder.Node, pts int64) {
		f := f1
		if n == n2 {
			f = f2
		}
		f.SetPts(pts)
		sw.HandleFrame(FrameHandlerPayload{Frame: f, Node: n})
	}

	type interceptedFrame struct {
		pts int64
		w   int
	}
	var interceptedFrames []interceptedFrame
	i := newFrameInterceptor(func(p FrameHandlerPayload) {
		interceptedFrames = append(interceptedFrames, interceptedFrame{
			pts: p.Frame.Pts(),
			w:   p.Frame.Width(),
		})

		go func(pts int64) {
			switch pts {
			case 3:
				sw.Switch(n2)
				sw.Switch(n1)
				dispatch(n1, 5)
			case 5:
				sw.Switch(n2)
				dispatch(n2, 5)
				dispatch(n2, 7)
				dispatch(n1, 6)
				dispatch(n2, 8)
			case 8:
				sw.Switch(n1)
				dispatch(n1, 7)
				dispatch(n1, 9)
				dispatch(n1, 10)
			case 10:
				sw.Switch(n2)
				dispatch(n2, 12)
				dispatch(n1, 11)
				dispatch(n2, 13)
			case 13:
				sw.Switch(n1)
				dispatch(n1, 15)
				dispatch(n2, 15)
				time.Sleep(15 * time.Millisecond)
				dispatch(n1, 16)
				wk.Stop()
			}
		}(p.Frame.Pts())
	}, eh, c, s)
	sw.Connect(i)

	var swiched []string
	eh.AddForEventName(EventNameSwitcherSwitchedOut, func(e astiencoder.Event) (deleteListener bool) {
		swiched = append(swiched, e.Payload.(astiencoder.Node).Metadata().Name)
		return
	})

	sw.Switch(n1)
	dispatch(n1, 3)

	sw.Start(wk.Context(), wk.NewTask)

	wk.Wait()

	require.Equal(t, []interceptedFrame{
		{pts: 3, w: 1},
		{pts: 4, w: 1},
		{pts: 5, w: 1},
		{pts: 6, w: 1},
		{pts: 7, w: 2},
		{pts: 8, w: 2},
		{pts: 9, w: 1},
		{pts: 10, w: 1},
		{pts: 11, w: 1},
		{pts: 12, w: 2},
		{pts: 13, w: 2},
		{pts: 14, w: 2},
		{pts: 15, w: 1},
		{pts: 16, w: 1},
	}, interceptedFrames)
	require.Equal(t, []string{"n1", "n2", "n1", "n2", "n1"}, swiched)
}
