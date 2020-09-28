package astilibav

// Event names
const (
	// First packet of new node has been received by the rate enforcer
	RateEnforcerSwitchedIn = "astilibav.rate.enforcer.switched.in"
	// First packet of new node has been dispatched by the rate enforcer
	RateEnforcerSwitchedOut = "astilibav.rate.enforcer.switched.out"
)

// Stat names
const (
	StatNameAverageDelay  = "astilibav.average.delay"
	StatNameDispatchRatio = "astilibav.dispatch.ratio"
	StatNameIncomingRate  = "astilibav.incoming.rate"
	StatNameRepeatedRate  = "astilibav.repeated.rate"
	StatNameWorkRatio     = "astilibav.work.ratio"
)
