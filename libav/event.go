package astilibav

// Event names
const (
	EventNameLog = "astilibav.log"
	// First frame has been dispatched by node
	EventNameFirstFrameOut = "astilibav.first.frame.out"
	// First frame of new node has been received by the rate enforcer
	EventNameRateEnforcerSwitchedIn = "astilibav.rate.enforcer.switched.in"
	// First frame of new node has been dispatched by the rate enforcer
	EventNameRateEnforcerSwitchedOut = "astilibav.rate.enforcer.switched.out"
)

// Stat names
const (
	StatNameAverageDelay  = "astilibav.average.delay"
	StatNameFilledRate    = "astilibav.filled.rate"
	StatNameIncomingRate  = "astilibav.incoming.rate"
	StatNameOutgoingRate  = "astilibav.outgoing.rate"
	StatNameProcessedRate = "astilibav.processed.rate"
	StatNameWorkRatio     = "astilibav.work.ratio"
)
