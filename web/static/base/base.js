const base = {
    init: function(websocketFunc, pageFunc) {
        // Init astitools
        asticode.loader.init()
        asticode.notifier.init()
        asticode.modaler.init()

        // Get references
        asticode.loader.show()
        asticode.tools.sendHttp({
            method: "GET",
            url: "/api/references",
            success: function(data) {
                asticode.ws.init({
                    okRequest: {
                        method: "GET",
                        url: "/api/ok"
                    },
                    url: (location.protocol === 'https:' ? "wss:" : "ws:") + "//" + window.location.host + "/websocket",
                    pingPeriod: data.responseJSON.ws_ping_period,
                    offline: function() { asticode.notifier.error("Server is offline") },
                    message: function (eventName, payload) {
                        // Handle menu events
                        menu.websocketFunc(eventName, payload)

                        // Custom function
                        if (typeof websocketFunc !== "undefined" && websocketFunc !== null) websocketFunc(eventName, payload)
                    },
                    open: function() {
                        asticode.tools.sendHttp({
                            method: "GET",
                            url: "/api/workflows",
                            error: function () {
                                asticode.loader.hide()
                            },
                            success: function(data) {
                                // Init menu
                                menu.init(data.responseJSON)

                                // Custom function
                                if (typeof pageFunc !== "undefined" && pageFunc !== null) {
                                    pageFunc(data)
                                } else {
                                    asticode.loader.hide()
                                }
                            }
                        })
                    },
                })
            },
            error: function() {
                asticode.loader.hide()
            },
        })
    },
    defaultHttpError: function(data) {
        asticode.notifier.error(data.responseJSON.message)
        asticode.loader.hide()
    },
}