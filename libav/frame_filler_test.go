package astilibav

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestFrameFiller(t *testing.T) {
	c := astikit.NewCloser()
	defer c.Close()
	eh := astiencoder.NewEventHandler()
	var logs []string
	eh.AddForEventName(astiencoder.EventNameError, func(e astiencoder.Event) (deleteListener bool) {
		t.Log(e.Payload)
		logs = append(logs, e.Payload.(error).Error())
		return
	})
	ff := NewFrameFiller(c, eh, nil)

	f, n := ff.Get()
	require.Nil(t, f)
	require.Nil(t, n)

	ff.WithPreviousFrame()
	nn := &Filterer{}
	_, err := ff.WithFallbackFrame(FrameFillerFallbackFrameOptions{
		FrameAdapter: EmptyVideoFrameAdapter(astiav.ColorRangeUnspecified, astiav.PixelFormatYuv420P, 4, 2),
		Node:         nn,
	})
	require.NoError(t, err)
	f, n = ff.Get()
	require.NotNil(t, f)
	require.Equal(t, n, nn)

	_, err = ff.WithFallbackFrame(FrameFillerFallbackFrameOptions{
		FrameAdapter: EmptyVideoFrameAdapter(astiav.ColorRangeUnspecified, astiav.PixelFormatYuv420P, 4, 2),
	})
	require.NoError(t, err)
	f, n = ff.Get()
	require.NotNil(t, f)
	require.Equal(t, 4, f.Width())
	require.Nil(t, n)

	nf := astiav.AllocFrame()
	defer nf.Free()
	ff.Put(nf, nil)
	require.Equal(t, []string{"astilibav: refing frame failed: Invalid argument"}, logs)
	f, n = ff.Get()
	require.NotNil(t, f)
	require.Equal(t, 4, f.Width())
	require.Nil(t, n)

	err = EmptyVideoFrameAdapter(astiav.ColorRangeUnspecified, astiav.PixelFormatYuv420P, 6, 2)(nf)
	require.NoError(t, err)
	ff.Put(nf, nn)

	for i := 0; i <= 1; i++ {
		f, n = ff.Get()
		require.NotNil(t, f)
		require.Equal(t, 6, f.Width())
		require.Equal(t, nn, n)
	}
}
