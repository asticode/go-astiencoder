const page = {
    init: function(name) {
        base.init(null, function() {
            asticode.tools.sendHttp({
                method: "GET",
                url: "/api/workflows/" + name,
                error: base.defaultHttpError,
                success: function(data) {
                    // Init network
                    page.initNetwork(data.responseJSON)

                    // Init job
                    page.initJob(data.responseJSON)

                    // Hide loading
                    asticode.loader.hide()
                }
            })

        })
    },
    initNetwork: function(data) {
        // Create graph description
        let desc = "graph LR\n"

        // Add nodes
        for (let idx = 0; idx < data.nodes.length; idx++) {
            desc += "    " + data.nodes[idx].name + "(" + data.nodes[idx].label + ")\n"
            desc += "    class " + data.nodes[idx].name + " " + (data.nodes[idx].is_stopped ? "stopped" : "started") + ";"
        }

        // Add edges
        for (let idx = 0; idx < data.edges.length; idx++) {
            desc += "    " + data.edges[idx].from + "-->" + data.edges[idx].to + "\n"
        }

        // Add graph description
        document.getElementById("network").innerHTML = desc

        // Initialize mermaid
        mermaid.init({}, ".network")
    },
    initJob: function(data) {
        document.getElementById("job").innerText = JSON.stringify(data.job, null, 4)
    },
}