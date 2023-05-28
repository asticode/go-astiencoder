package astilibav

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdaptiveDelayer(t *testing.T) {
	_now := now
	defer func() { now = _now }()
	var count int64
	now = func() time.Time {
		count++
		return time.Unix(count, 0)
	}

	d := NewAdaptiveDelayer(AdaptiveDelayerOptions{
		LookBehind:      500 * time.Millisecond,
		MarginGoingDown: 150 * time.Millisecond,
		MarginGoingUp:   50 * time.Millisecond,
		Maximum:         2 * time.Second,
		Start:           400 * time.Millisecond,
		Step:            400 * time.Millisecond,
	})

	for _, v := range []struct {
		expected time.Duration
		input    time.Duration
	}{
		{
			expected: 400 * time.Millisecond,
			input:    349 * time.Millisecond,
		},
		{
			expected: 800 * time.Millisecond,
			input:    351 * time.Millisecond,
		},
		{
			expected: 800 * time.Millisecond,
			input:    349 * time.Millisecond,
		},
		{
			expected: 800 * time.Millisecond,
			input:    251 * time.Millisecond,
		},
		{
			expected: 400 * time.Millisecond,
			input:    249 * time.Millisecond,
		},
		{
			expected: 400 * time.Millisecond,
			input:    349 * time.Millisecond,
		},
		{
			expected: 800 * time.Millisecond,
			input:    351 * time.Millisecond,
		},
		{
			expected: 1200 * time.Millisecond,
			input:    751 * time.Millisecond,
		},
		{
			expected: 2 * time.Second,
			input:    1951 * time.Millisecond,
		},
		{
			expected: 400 * time.Millisecond,
			input:    -10 * time.Millisecond,
		},
	} {
		d.HandleFrame(v.input, nil)
		require.Equal(t, v.expected, d.Delay())
	}
}
