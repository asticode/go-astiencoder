package astilibav

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAverageAdaptiveDelayer(t *testing.T) {
	_now := now
	defer func() { now = _now }()
	var count int64
	now = func() time.Time {
		count++
		return time.Unix(count, 0)
	}

	d := NewAdaptiveDelayer(AdaptiveDelayerOptions{
		Handler: AdaptiveDelayerHandlerOptions{Average: &AdaptiveDelayerAverageHandlerOptions{
			LookBehind:     500 * time.Millisecond,
			MarginDecrease: 150 * time.Millisecond,
			MarginIncrease: 50 * time.Millisecond,
		}},
		Minimum:    400 * time.Millisecond,
		Step:       400 * time.Millisecond,
		StepsCount: 5,
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

func TestAverageLosslessDelayer(t *testing.T) {
	_now := now
	defer func() { now = _now }()
	var n time.Time
	now = func() time.Time { return n }

	d := NewAdaptiveDelayer(AdaptiveDelayerOptions{
		Handler: AdaptiveDelayerHandlerOptions{Lossless: &AdaptiveDelayerLosslessHandlerOptions{
			LookBehind: 2 * time.Second,
		}},
		Minimum:    400 * time.Millisecond,
		Step:       400 * time.Millisecond,
		StepsCount: 5,
	})

	for idx, v := range []struct {
		expected time.Duration
		input    time.Duration
	}{
		{
			expected: 400 * time.Millisecond,
			input:    399 * time.Millisecond,
		},
		{
			expected: 800 * time.Millisecond,
			input:    401 * time.Millisecond,
		},
		{
			expected: 800 * time.Millisecond,
			input:    399 * time.Millisecond,
		},
		{
			expected: 400 * time.Millisecond,
			input:    399 * time.Millisecond,
		},
		{
			expected: 1200 * time.Millisecond,
			input:    801 * time.Millisecond,
		},
		{
			expected: 2 * time.Second,
			input:    3 * time.Second,
		},
		{
			expected: 2 * time.Second,
			input:    300 * time.Millisecond,
		},
		{
			expected: 400 * time.Millisecond,
			input:    300 * time.Millisecond,
		},
	} {
		n = time.Unix(int64(idx), 0)
		d.HandleFrame(v.input, nil)
		require.Equal(t, v.expected, d.Delay(), "idx: %d - input: %s", idx, v.input)
	}

	d = NewAdaptiveDelayer(AdaptiveDelayerOptions{
		Handler: AdaptiveDelayerHandlerOptions{Lossless: &AdaptiveDelayerLosslessHandlerOptions{
			DisableDecrease: true,
		}},
		Minimum:    400 * time.Millisecond,
		Step:       400 * time.Millisecond,
		StepsCount: 5,
	})

	for idx, v := range []struct {
		expected time.Duration
		input    time.Duration
	}{
		{
			expected: 400 * time.Millisecond,
			input:    399 * time.Millisecond,
		},
		{
			expected: 800 * time.Millisecond,
			input:    401 * time.Millisecond,
		},
		{
			expected: 800 * time.Millisecond,
			input:    399 * time.Millisecond,
		},
		{
			expected: 800 * time.Millisecond,
			input:    399 * time.Millisecond,
		},
		{
			expected: 1200 * time.Millisecond,
			input:    801 * time.Millisecond,
		},
		{
			expected: 2 * time.Second,
			input:    3 * time.Second,
		},
		{
			expected: 2 * time.Second,
			input:    300 * time.Millisecond,
		},
		{
			expected: 2 * time.Second,
			input:    300 * time.Millisecond,
		},
	} {
		n = time.Unix(int64(idx), 0)
		d.HandleFrame(v.input, nil)
		require.Equal(t, v.expected, d.Delay(), "idx: %d - input: %s", idx, v.input)
	}
}
