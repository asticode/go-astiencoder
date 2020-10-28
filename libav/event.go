package astilibav

// Event names
const (
	EventNameLog = "astilibav.log"
	// First packet of new node has been received by the rate enforcer
	EventNameRateEnforcerSwitchedIn = "astilibav.rate.enforcer.switched.in"
	// First packet of new node has been dispatched by the rate enforcer
	EventNameRateEnforcerSwitchedOut = "astilibav.rate.enforcer.switched.out"
)

// Stat names
const (
	StatNameAverageDelay  = "astilibav.average.delay"
	StatNameIncomingRate  = "astilibav.incoming.rate"
	StatNameOutgoingRate  = "astilibav.outgoing.rate"
	StatNameProcessedRate = "astilibav.processed.rate"
	StatNameRepeatedRate  = "astilibav.repeated.rate"
	StatNameWorkRatio     = "astilibav.work.ratio"
)
