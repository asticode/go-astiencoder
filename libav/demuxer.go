package astilibav

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

var (
	countDemuxer       uint64
	NanosecondRational = astiav.NewRational(1, 1e9)
)

// Demuxer represents an object capable of demuxing packets out of an input
type Demuxer struct {
	*astiencoder.BaseNode
	d                     *pktDispatcher
	eh                    *astiencoder.EventHandler
	er                    *demuxerEmulateRate
	flushOnStart          bool
	formatContext         *astiav.FormatContext
	ioInterrupter         *astiav.IOInterrupter
	l                     *demuxerLoop
	p                     *pktPool
	pb                    *demuxerProbe
	readFrameErrorHandler DemuxerReadFrameErrorHandler
	ss                    map[int]*demuxerStream
	statBytesRead         uint64
}

// Demuxer will start by dispatching without sleeping all packets with negative PTS
// followed by n seconds of packets so that next nodes (e.g. the decoder) have a sufficient
// buffer to do their work properly (e.g. decoders don't output frames until their PTS is
// positive). After that, Demuxer sleeps between packets based on their DTS.
type DemuxerEmulateRateOptions struct {
	// BufferDuration represents the duration of packets with positive PTS dispatched at the
	// start without sleeping.
	// Defaults to 1s
	BufferDuration time.Duration
	Enabled        bool
}

type demuxerEmulateRate struct {
	bufferDuration time.Duration
	enabled        bool
}

func newDemuxerEmulateRate(o DemuxerEmulateRateOptions) *demuxerEmulateRate {
	r := &demuxerEmulateRate{
		bufferDuration: o.BufferDuration,
		enabled:        o.Enabled,
	}
	if r.bufferDuration <= 0 {
		r.bufferDuration = time.Second
	}
	return r
}

// Demuxer will seek back to the start of the input when eof is reached
// In this case the packets are restamped
type DemuxerLoopOptions struct {
	Enabled bool
}

type demuxerLoop struct {
	// Number of time it has looped
	cycleCount uint
	// Duration of one loop cycle
	cycleDuration time.Duration
	enabled       uint32
}

func newDemuxerLoop(o DemuxerLoopOptions) *demuxerLoop {
	return &demuxerLoop{enabled: astikit.BoolToUInt32(o.Enabled)}
}

type DemuxerProbeInfo struct {
	FirstPTS DemuxerProbeInfoFirstPTS
}

type DemuxerProbeInfoFirstPTS struct {
	// Streams whose first pts is the same as the overall first pts.
	// Indexed by stream index
	Streams  map[int]bool
	Timebase astiav.Rational
	Value    int64
}

func (i DemuxerProbeInfo) PTSReference() *PTSReference {
	return NewPTSReference().Update(i.FirstPTS.Value, time.Now(), i.FirstPTS.Timebase)
}

type demuxerProbe struct {
	data     []*astiav.Packet
	duration time.Duration
	info     *DemuxerProbeInfo
}

func newDemuxerProbe(duration time.Duration) *demuxerProbe {
	p := &demuxerProbe{duration: duration}
	if p.duration <= 0 {
		p.duration = time.Second
	}
	return p
}

type demuxerStreamEmulateRate struct {
	bufferDuration time.Duration
	referenceTime  time.Time
	// In stream timebase
	referenceTS int64
}

func (d *Demuxer) newDemuxerStreamEmulateRate() *demuxerStreamEmulateRate {
	return &demuxerStreamEmulateRate{bufferDuration: d.er.bufferDuration}
}

type demuxerStreamLoop struct {
	cycleFirstPktPTS          *int64
	cycleFirstPktPTSRemainder time.Duration
	cycleLastPktDuration      time.Duration
	cycleLastPktPTS           int64
	restampRemainder          time.Duration
}

func newDemuxerStreamLoop() *demuxerStreamLoop {
	return &demuxerStreamLoop{}
}

type demuxerStream struct {
	ctx Context
	d   Descriptor
	er  *demuxerStreamEmulateRate
	l   *demuxerStreamLoop
	s   *astiav.Stream
}

func (d *Demuxer) newDemuxerStream(s *astiav.Stream) *demuxerStream {
	// Create ctx
	ctx := NewContextFromStream(s)

	// Create demuxer stream
	return &demuxerStream{
		ctx: ctx,
		d:   ctx.Descriptor(),
		er:  d.newDemuxerStreamEmulateRate(),
		l:   newDemuxerStreamLoop(),
		s:   s,
	}
}

func (d *demuxerStream) stream() *Stream {
	return &Stream{
		CodecParameters: d.s.CodecParameters(),
		Ctx:             d.ctx,
		ID:              d.s.ID(),
		Index:           d.s.Index(),
	}
}

type DemuxerReadFrameErrorHandler func(d *Demuxer, err error) (stop, handled bool)

// DemuxerOptions represents demuxer options
type DemuxerOptions struct {
	// String content of the demuxer as you would use in ffmpeg
	Dictionary *Dictionary
	// Emulate rate options
	EmulateRate DemuxerEmulateRateOptions
	// If true, flushes internal data on start
	FlushOnStart bool
	// Exact input format
	Format *astiav.InputFormat
	// IO Context to use
	IOContext *astiav.IOContext
	// Loop options
	Loop DemuxerLoopOptions
	// Basic node options
	Node astiencoder.NodeOptions
	// Context used to cancel probing
	ProbeCtx context.Context
	// In order to emulate rate or loop properly, Demuxer needs to probe data.
	// ProbeDuration represents the duration the Demuxer will probe.
	// Defaults to 1s
	ProbeDuration time.Duration
	// Custom read frame error handler
	// If handled is false, default error handling will be executed
	ReadFrameErrorHandler DemuxerReadFrameErrorHandler
	// URL of the input
	URL string
}

// NewDemuxer creates a new demuxer
func NewDemuxer(o DemuxerOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (d *Demuxer, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countDemuxer, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("demuxer_%d", count), fmt.Sprintf("Demuxer #%d", count), fmt.Sprintf("Demuxes %s", o.URL), "demuxer")

	// Create demuxer
	d = &Demuxer{
		eh:                    eh,
		er:                    newDemuxerEmulateRate(o.EmulateRate),
		flushOnStart:          o.FlushOnStart,
		l:                     newDemuxerLoop(o.Loop),
		pb:                    newDemuxerProbe(o.ProbeDuration),
		readFrameErrorHandler: o.ReadFrameErrorHandler,
		ss:                    make(map[int]*demuxerStream),
	}

	// Create base node
	d.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, d, astiencoder.EventTypeToNodeEventName)

	// Create pkt pool
	d.p = newPktPool(d)

	// Create pkt dispatcher
	d.d = newPktDispatcher(d, eh)

	// Add stat options
	d.addStatOptions()

	// Dictionary
	var dict *astiav.Dictionary
	if o.Dictionary != nil {
		// Parse dict
		if dict, err = o.Dictionary.parse(); err != nil {
			err = fmt.Errorf("astilibav: parsing dict failed: %w", err)
			return
		}

		// Make sure the dictionary is freed
		defer dict.Free()
	}

	// Alloc format context
	d.formatContext = astiav.AllocFormatContext()

	// Make sure the format context is properly freed
	d.AddClose(d.formatContext.Free)

	// Set io interrupt
	d.ioInterrupter = astiav.NewIOInterrupter()
	d.formatContext.SetIOInterrupter(d.ioInterrupter)

	// Handle probe cancellation
	if o.ProbeCtx != nil {
		// Create context
		probeCtx, probeCancel := context.WithCancel(o.ProbeCtx)

		// Handle interrupt
		d.ioInterrupter.Resume()
		go func() {
			<-probeCtx.Done()
			if o.ProbeCtx.Err() != nil {
				d.ioInterrupter.Interrupt()
			}
		}()

		// Make sure to cancel context so that go routine is closed
		defer probeCancel()
	}

	// No url but an io context, we need to set the pb before opening the input
	if o.URL == "" && o.IOContext != nil {
		d.formatContext.SetPb(o.IOContext)
	}

	// Open input
	if err = d.formatContext.OpenInput(o.URL, o.Format, dict); err != nil {
		err = fmt.Errorf("astilibav: opening input failed: %w", err)
		return
	}

	// Make sure the input is properly closed
	d.AddClose(d.formatContext.CloseInput)

	// An url and an io context, we need to set the pb after opening the input
	// Make sure the previous pb is closed
	if o.URL != "" && o.IOContext != nil {
		if pb := d.formatContext.Pb(); pb != nil {
			pb.Close()
		}
		d.formatContext.SetPb(o.IOContext)
		d.formatContext.SetFlags(d.formatContext.Flags().Add(astiav.FormatContextFlagCustomIo))
	}

	// Check whether probe has been cancelled
	if o.ProbeCtx != nil && o.ProbeCtx.Err() != nil {
		err = fmt.Errorf("astilibav: probing has been cancelled: %w", o.ProbeCtx.Err())
		return
	}

	// Find stream information
	if err = d.formatContext.FindStreamInfo(nil); err != nil {
		err = fmt.Errorf("astilibav: finding stream info failed: %w", err)
		return
	}

	// Check whether probe has been cancelled
	if o.ProbeCtx != nil && o.ProbeCtx.Err() != nil {
		err = fmt.Errorf("astilibav: probing has been cancelled: %w", o.ProbeCtx.Err())
		return
	}

	// Create streams
	for _, s := range d.formatContext.Streams() {
		d.ss[s.Index()] = d.newDemuxerStream(s)
	}

	// Probe
	if d.er.enabled || atomic.LoadUint32(&d.l.enabled) > 0 {
		if err = d.probe(); err != nil {
			err = fmt.Errorf("astilibav: probing failed: %w", err)
			return
		}
	}
	return
}

// Probes the starting pkts of a duration equivalent to probeDuration to retrieve
// the first overall PTS and the streams whose first PTS is the same as the first
// overall PTS
// We don't stop before probeDuration in case the smallest PTS is not in the
// first pkt.
func (d *Demuxer) probe() (err error) {
	// Loop
	firstPTSs := make(map[*demuxerStream]int64)
	for {
		// Get pkt from pool
		pkt := d.p.get()

		// Read frame
		if errReadFrame := d.formatContext.ReadFrame(pkt); errReadFrame != nil {
			// Make sure to close pkt
			d.p.put(pkt)

			// We've reached eof but we have enough information
			if errors.Is(errReadFrame, astiav.ErrEof) && len(firstPTSs) > 0 {
				break
			}

			// We don't have enough information
			err = fmt.Errorf("asilibav: reading frame failed: %w", errReadFrame)
			return
		}

		// Add pkt to probe data
		d.pb.data = append(d.pb.data, pkt)

		// Invalid timestamps
		// Only frames with PTS >= 0 get out of decoders
		if pkt.Pts() == astiav.NoPtsValue || pkt.Pts() < 0 {
			continue
		}

		// Get stream
		s, ok := d.ss[pkt.StreamIndex()]
		if !ok {
			continue
		}

		// Get pts
		pts := pkt.Pts()

		// Process pkt side data
		if skippedStart, _ := d.processPktSideData(pkt, s); skippedStart > 0 {
			// Get duration
			sd, _ := durationToTimeBase(skippedStart, s.ctx.TimeBase)

			// Update pts
			pts += sd
		}

		// Update first pts
		if firstPTS, ok := firstPTSs[s]; ok {
			if pts < firstPTS {
				firstPTSs[s] = pts
			}
		} else {
			firstPTSs[s] = pts
		}

		// We've reached probe duration
		if time.Duration(astiav.RescaleQ(pts-firstPTSs[s], s.ctx.TimeBase, NanosecondRational)) > d.pb.duration {
			break
		}
	}

	// Get first overall PTS in nanosecond timebase
	var firstPTS *int64
	for s, v := range firstPTSs {
		pts := astiav.RescaleQ(v, s.ctx.TimeBase, NanosecondRational)
		if firstPTS == nil {
			firstPTS = astikit.Int64Ptr(pts)
		} else if pts < *firstPTS {
			*firstPTS = pts
		}
	}

	// Update probe info
	d.pb.info = &DemuxerProbeInfo{FirstPTS: DemuxerProbeInfoFirstPTS{
		Streams:  make(map[int]bool),
		Timebase: NanosecondRational,
		Value:    *firstPTS,
	}}
	for s, v := range firstPTSs {
		pts := astiav.RescaleQ(v, s.ctx.TimeBase, NanosecondRational)
		if pts == *firstPTS {
			d.pb.info.FirstPTS.Streams[s.s.Index()] = true
		}
	}

	// Update streams emulate rate reference timestamp
	for _, s := range d.ss {
		s.er.referenceTS = astiav.RescaleQ(*firstPTS, NanosecondRational, s.ctx.TimeBase)
	}
	return
}

type DemuxerStats struct {
	BytesRead         uint64
	PacketsAllocated  uint64
	PacketsDispatched uint64
}

func (d *Demuxer) Stats() DemuxerStats {
	return DemuxerStats{
		BytesRead:         atomic.LoadUint64(&d.statBytesRead),
		PacketsAllocated:  d.p.stats().packetsAllocated,
		PacketsDispatched: d.d.stats().packetsDispatched,
	}
}

func (d *Demuxer) addStatOptions() {
	// Get stats
	ss := d.d.statOptions()
	ss = append(ss, d.p.statOptions()...)
	ss = append(ss, astikit.StatOptions{
		Metadata: &astikit.StatMetadata{
			Description: "Number of bytes read per second",
			Label:       "Read rate",
			Name:        StatNameReadRate,
			Unit:        "Bps",
		},
		Valuer: astikit.NewAtomicUint64RateStat(&d.statBytesRead),
	})

	// Add stats
	d.BaseNode.AddStats(ss...)
}

func (d *Demuxer) ProbeInfo() *DemuxerProbeInfo {
	return d.pb.info
}

func (d *Demuxer) SetLoop(loop bool) {
	atomic.StoreUint32(&d.l.enabled, astikit.BoolToUInt32(loop))
}

// Streams returns the streams ordered by index
func (d *Demuxer) Streams() (ss []*Stream) {
	// Get indexes
	var idxs []int
	for idx := range d.ss {
		idxs = append(idxs, idx)
	}

	// Sort indexes
	sort.Ints(idxs)

	// Loop through indexes
	for _, idx := range idxs {
		ss = append(ss, d.ss[idx].stream())
	}
	return
}

// Connect implements the PktHandlerConnector interface
func (d *Demuxer) Connect(h PktHandler) {
	// Add handler
	d.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(d, h)
}

// Disconnect implements the PktHandlerConnector interface
func (d *Demuxer) Disconnect(h PktHandler) {
	// Delete handler
	d.d.delHandler(h)

	// Disconnect nodes
	astiencoder.DisconnectNodes(d, h)
}

// ConnectForStream connects the demuxer to a PktHandler for a specific stream
func (d *Demuxer) ConnectForStream(h PktHandler, i *Stream) {
	// Add handler
	d.d.addHandler(newPktCond(i, h))

	// Connect nodes
	astiencoder.ConnectNodes(d, h)
}

// DisconnectForStream disconnects the demuxer from a PktHandler for a specific stream
func (d *Demuxer) DisconnectForStream(h PktHandler, i *Stream) {
	// Delete handler
	d.d.delHandler(newPktCond(i, h))

	// Disconnect nodes
	astiencoder.DisconnectNodes(d, h)
}

// Start starts the demuxer
func (d *Demuxer) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	d.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Handle io interrupt
		d.ioInterrupter.Resume()
		go func() {
			<-d.Context().Done()
			d.ioInterrupter.Interrupt()
		}()

		// Update emulate rate time references
		if d.er.enabled {
			// Create reference time
			referenceTime := time.Now().Add(-d.er.bufferDuration)

			// Loop through streams
			for _, s := range d.ss {
				// Update stream reference time
				if s.er.referenceTime.IsZero() {
					s.er.referenceTime = referenceTime
				}
			}
		}

		// Flush
		if d.flushOnStart {
			if err := d.formatContext.Flush(); err != nil {
				emitError(d, d.eh, err, "flushing")
			}
		}

		// Loop
		for {
			// Read frame
			if stop := d.readFrame(); stop {
				break
			}

			// Handle pause
			d.HandlePause()

			// Check context
			if d.Context().Err() != nil {
				break
			}
		}
	})
}

func (d *Demuxer) nextPkt() (pkt *astiav.Packet, handle, stop bool) {
	// Check probe data first
	if len(d.pb.data) > 0 {
		pkt = d.pb.data[0]
		d.pb.data = d.pb.data[1:]
		handle = true
		return
	}

	// Get pkt from pool
	pkt = d.p.get()

	// Read frame
	if err := d.formatContext.ReadFrame(pkt); err != nil {
		if atomic.LoadUint32(&d.l.enabled) > 0 && errors.Is(err, astiav.ErrEof) {
			// Loop
			d.loop()

			// Get seek information
			seekStreamIdx := -1
			seekTimestamp := d.formatContext.StartTime()
			if d.pb.info != nil {
				// Loop through streams
				for idx := range d.pb.info.FirstPTS.Streams {
					// Get stream
					s, ok := d.ss[idx]
					if !ok {
						continue
					}

					// Update
					seekStreamIdx = s.ctx.Index
					seekTimestamp = astiav.RescaleQ(d.pb.info.FirstPTS.Value, d.pb.info.FirstPTS.Timebase, s.ctx.TimeBase)
					break
				}
			}

			// Seek to start
			if err = d.formatContext.SeekFrame(seekStreamIdx, seekTimestamp, astiav.NewSeekFlags(astiav.SeekFlagBackward)); err != nil {
				emitError(d, d.eh, err, "seeking to frame")
				stop = true
			}
		} else {
			// Custom error handler
			if d.readFrameErrorHandler != nil {
				var handled bool
				if stop, handled = d.readFrameErrorHandler(d, err); handled {
					return
				}
			}

			// Default error handling
			if !errors.Is(err, astiav.ErrEof) {
				emitError(d, d.eh, err, "reading frame")
			}
			stop = true
		}
		return
	}

	// Pkt should be handled
	handle = true
	return
}

func (d *Demuxer) readFrame() bool {
	// Get next pkt
	pkt, handle, stop := d.nextPkt()

	// First, make sure pkt is properly closed
	defer d.p.put(pkt)

	// Stop
	if stop {
		return true
	} else if !handle {
		return false
	}

	// Increment read bytes
	atomic.AddUint64(&d.statBytesRead, uint64(pkt.Size()))

	// Handle pkt
	d.handlePkt(pkt)
	return false
}

func (d *Demuxer) handlePkt(pkt *astiav.Packet) {
	// Get stream
	s, ok := d.ss[pkt.StreamIndex()]
	if !ok {
		return
	}

	// Timestamps are valid
	if pkt.Dts() != astiav.NoPtsValue && pkt.Pts() != astiav.NoPtsValue {
		// Process pkt duration
		// Do it before processing side data
		// Since we can't get more precise than nanoseconds, if there's precision loss here, there's nothing
		// we can do about it
		if d.l.cycleCount == 0 {
			s.l.cycleLastPktDuration = time.Duration(astiav.RescaleQ(pkt.Duration(), s.ctx.TimeBase, NanosecondRational))
		}

		// Process pkt side data
		skippedStart, skippedEnd := d.processPktSideData(pkt, s)

		// Skipped start
		if skippedStart > 0 {
			// Get duration
			sd, sr := durationToTimeBase(skippedStart, s.ctx.TimeBase)

			// Restamp
			pkt.SetDts(pkt.Dts() + sd)
			pkt.SetPts(pkt.Pts() + sd)

			// Store remainder
			if d.l.cycleCount == 0 {
				// Only frames with PTS >= 0 get out of decoders
				if s.l.cycleFirstPktPTS == nil && pkt.Pts() >= 0 {
					s.l.cycleFirstPktPTSRemainder = sr
				}
			}
		}

		// Skipped end
		if skippedEnd > 0 {
			if d.l.cycleCount == 0 {
				s.l.cycleLastPktDuration -= skippedEnd
			}
		}

		// Process pkt pts
		// Do it after processing side data
		if d.l.cycleCount == 0 {
			// Only frames with PTS >= 0 get out of decoders
			if s.l.cycleFirstPktPTS == nil && pkt.Pts() >= 0 {
				s.l.cycleFirstPktPTS = astikit.Int64Ptr(pkt.Pts())
			}
			s.l.cycleLastPktPTS = pkt.Pts()
		}

		// Loop restamp
		if atomic.LoadUint32(&d.l.enabled) > 0 && d.l.cycleCount > 0 {
			// Get duration
			var dl int64
			dl, s.l.restampRemainder = durationToTimeBase(time.Duration(d.l.cycleCount)*d.l.cycleDuration+s.l.restampRemainder, s.ctx.TimeBase)

			// Restamp
			pkt.SetDts(pkt.Dts() + dl)
			pkt.SetPts(pkt.Pts() + dl)
		}

		// Emulate rate
		if d.er.enabled {
			// Get pkt at
			pktAt := s.er.referenceTime.Add(time.Duration(astiav.RescaleQ(pkt.Dts()-s.er.referenceTS, s.ctx.TimeBase, NanosecondRational)))

			// Wait if there are too many pkts in rate emulator buffer
			if delta := time.Until(pktAt) - s.er.bufferDuration; delta > 0 {
				astikit.Sleep(d.Context(), delta) //nolint:errcheck
			}
		}
	}

	// Dispatch pkt
	d.d.dispatch(pkt, s.d)
}

func (d *Demuxer) processPktSideData(pkt *astiav.Packet, s *demuxerStream) (skippedStart, skippedEnd time.Duration) {
	// Switch on media type
	switch s.ctx.MediaType {
	case astiav.MediaTypeAudio:
		skippedStart, skippedEnd = d.processPktSideDataSkipSamples(pkt, s)
	}
	return
}

func (d *Demuxer) processPktSideDataSkipSamples(pkt *astiav.Packet, s *demuxerStream) (skippedStart, skippedEnd time.Duration) {
	// Get skip samples side data
	sd := pkt.SideData().Get(astiav.PacketSideDataTypeSkipSamples)
	if sd == nil {
		return
	}

	// Get number of skipped samples
	skippedSamplesStart, skippedSamplesEnd := astiav.RL32WithOffset(sd, 0), astiav.RL32WithOffset(sd, 4)

	// Skipped start
	if skippedSamplesStart > 0 {
		skippedStart = time.Duration(float64(skippedSamplesStart) / float64(s.ctx.SampleRate) * 1e9)
	}

	// Skipped end
	if skippedSamplesEnd > 0 {
		skippedEnd = time.Duration(float64(skippedSamplesEnd) / float64(s.ctx.SampleRate) * 1e9)
	}
	return
}

func (d *Demuxer) loop() {
	// This is the first time it's looping
	if d.l.cycleCount == 0 {
		// Loop through streams
		for _, s := range d.ss {
			// No first pkt pts
			if s.l.cycleFirstPktPTS == nil {
				continue
			}

			// Get duration
			// Since we can't get more precise than nanoseconds, if there's precision loss here, there's nothing
			// we can do about it
			ld := s.l.cycleFirstPktPTSRemainder + s.l.cycleLastPktDuration + time.Duration(astiav.RescaleQ(s.l.cycleLastPktPTS-*s.l.cycleFirstPktPTS, s.ctx.TimeBase, NanosecondRational))

			// Update loop cycle duration
			if d.l.cycleDuration < ld {
				d.l.cycleDuration = ld
			}
		}
	}

	// Increment loop cycle count
	d.l.cycleCount++
}
