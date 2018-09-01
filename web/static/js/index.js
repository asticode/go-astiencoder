const index = {
    init: function() {
        // Init ws
        this.initWs();
    },
    initWs: function() {
        const self = this
        this.ws = new WebSocket("ws://" + window.location.host + "/websocket")
        this.ws.onerror = function(e) {
            console.error("ws error", e)
        }
        this.ws.onclose = function() {
            console.info("ws closed on " + self.ws.url)
        }
        this.ws.onmessage = function(e) {
            console.debug("ws onmessage", e)
        }
        this.ws.onopen = function() {
            console.info("ws opened on " + self.ws.url)
        }
    }
}