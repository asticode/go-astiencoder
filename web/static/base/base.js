const base = {
    init: function(websocketFunc, pageFunc) {
        // Init astitools
        asticode.loader.init()
        asticode.notifier.init()
        asticode.modaler.init()

        // Init buttons
        base.initButtons()

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
                    url: "ws://" + window.location.host + "/websocket",
                    pingPeriod: data.responseJSON.ws_ping_period,
                    offline: function() { asticode.notifier.error("Encoder is offline") },
                    message: function (eventName, payload) {
                        // TODO Handle menu events

                        if (typeof websocketFunc !== "undefined" && websocketFunc !== null) websocketFunc(data.event_name, data.payload)
                    },
                    open: function() {
                        asticode.tools.sendHttp({
                            method: "GET",
                            url: "/api/encoder",
                            error: function () {
                                asticode.loader.hide()
                            },
                            success: function(data) {
                                // Init menu
                                menu.init(data.responseJSON)

                                // Custom function
                                if (typeof pageFunc !== "undefined" && pageFunc !== null) pageFunc(data)
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
    initButtons: function() {
        this.initButtonAddWorkflow()
        this.initButtonStopEncoder()
    },
    initButtonAddWorkflow: function () {
        document.getElementById("btn-workflow-add").onclick = function() {
            const m = new Modal()
            m.addTitle("Add a workflow")
            m.addError()
            m.addLabel("Name:")
            const name = m.addInputText()
            m.addLabel("Job:")
            const job = m.addInputFile()
            m.addSubmit("Add", function() {
                m.resetError()
                const form = new FormData()
                form.append("name", name.value)
                if (job.files.length > 0) {
                    form.append("job", job.files[0])
                }
                asticode.tools.sendHttp({
                    method: "POST",
                    url: "/api/workflows",
                    payload: form,
                    error: function(data) {
                        if (typeof data.responseJSON.message !== "undefined") m.setError(data.responseJSON.message)
                    },
                    success: function() {
                        window.location = "/web/workflow?name=" + encodeURIComponent(name.value)
                    }
                })
            })
            asticode.modaler.setContent(m.wrapper)
            asticode.modaler.setWidth("500px")
            asticode.modaler.show()
        }
    },
    initButtonStopEncoder: function() {
        document.getElementById("btn-encoder-stop").onclick = function() {
            asticode.tools.sendHttp("/api/encoder/stop", "GET")
        }
    },
    defaultHttpError: function(data) {
        asticode.notifier.error(data.responseJSON.message)
        asticode.loader.hide()
    }
}