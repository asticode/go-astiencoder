package astilibav

// Event names
const (
	EventNameLog = "astilibav.log"
	// First frame of new node has been dispatched by the rate enforcer
	EventNameRateEnforcerSwitchedOut = "astilibav.rate.enforcer.switched.out"
	EventNameSwitcherSwitchedOut     = "astilibav.switcher.switched.out"
)

// Stat names
const (
	StatNameAllocatedFrames   = "astilibav.allocated.frames"
	StatNameAllocatedPackets  = "astilibav.allocated.packets"
	StatNameAverageDelay      = "astilibav.average.delay"
	StatNameAverageFrameDelay = "astilibav.average.frame.delay"
	StatNameFilledRate        = "astilibav.filled.rate"
	StatNameIncomingRate      = "astilibav.incoming.rate"
	StatNameOutgoingRate      = "astilibav.outgoing.rate"
	StatNameProcessedRate     = "astilibav.processed.rate"
	StatNameReadRate          = "astilibav.read.rate"
	StatNameWrittenRate       = "astilibav.written.rate"
)
