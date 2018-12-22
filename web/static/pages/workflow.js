const page = {
    nodes: {},
    init: function(name) {
        base.init(this.websocketFunc, function() {
            asticode.tools.sendHttp({
                method: "GET",
                url: "/api/workflows/" + name,
                error: base.defaultHttpError,
                success: function(data) {
                    // Store workflow name
                    page.workflow = name

                    // Init network
                    page.initNetwork(data.responseJSON)

                    // Hide loading
                    asticode.loader.hide()
                }
            })

        })
    },
    initNetwork: function(data) {
        // Handle node click
        window.handleNodeClick = function(name) {
            // Get node
            const node = page.nodes[name]

            // Node doesn't exist
            if (typeof node === "undefined") return

            // Get action
            let action = "start"
            switch (node.status) {
                case "running":
                    action = "pause"
                    break
                case "paused":
                    action = "continue"
                    break
            }

            // Send order to API
            asticode.tools.sendHttp({
                method: "GET",
                url: "/api/workflows/" + page.workflow + "/nodes/" + name + "/" + action,
                error: base.defaultHttpError,
            })
        }

        // Create graph description
        let desc = "graph TB\n"

        // Add nodes
        for (let idx = 0; idx < data.nodes.length; idx++) {
            // Get stats
            let stats = ""
            if (data.nodes[idx].stats.length > 0) {
                stats += "<br><br><table>"
                for (let idxStat = 0; idxStat < data.nodes[idx].stats.length; idxStat++) {
                    stats += "<tr><td>" + data.nodes[idx].stats[idxStat].label + ":</td><td><span></span>" + data.nodes[idx].stats[idxStat].unit + "</td>"
                }
                stats += "</table>"
            }

            // Add node graph description
            desc += "    " + data.nodes[idx].name + "(\"" + data.nodes[idx].label + stats + "\")\n"
            desc += "    class " + data.nodes[idx].name + " " + data.nodes[idx].status + ";"
            desc += "    click " + data.nodes[idx].name + " handleNodeClick;"

            // Add node to pool
            page.nodes[data.nodes[idx].name] = {
                status: data.nodes[idx].status
            }
        }

        // Add edges
        for (let idx = 0; idx < data.edges.length; idx++) {
            desc += "    " + data.edges[idx].from + "-->" + data.edges[idx].to + "\n"
        }

        // Add graph description
        document.getElementById("network").innerHTML = desc
        document.getElementById("network").removeAttribute("data-processed")

        // Initialize mermaid
        mermaid.init({}, ".network")
    },
    websocketFunc: function(eventName, payload) {
        switch (eventName) {
            case "node.continued":
            case "node.paused":
            case "node.started":
            case "node.stopped":
                // Get node
                const node = document.getElementById(payload)

                // Node doesn't exist
                if (typeof node === "undefined") return

                // Update class
                let status
                switch (eventName) {
                    case "node.continued":
                        status = "running"
                        asticode.tools.removeClass(node, "paused")
                        asticode.tools.addClass(node, status)
                        break
                    case "node.paused":
                        status = "paused"
                        asticode.tools.removeClass(node, "running")
                        asticode.tools.addClass(node, status)
                        break
                    case "node.started":
                        status = "running"
                        asticode.tools.removeClass(node, "stopped")
                        asticode.tools.addClass(node, status)
                        break
                    case "node.stopped":
                        status = "stopped"
                        asticode.tools.removeClass(node, "running")
                        asticode.tools.addClass(node, status)
                        break
                }

                // Update status
                if (typeof page.nodes[payload] !== "undefined") page.nodes[payload].status = status
                break
            case "stats":
                console.info(payload)
                // Get element
                const el = document.getElementById(payload.name)

                // Element doesn't exist
                if (typeof el === "undefined") return

                // Get lines
                const trs = el.querySelectorAll("tr")

                // Element doesn't exist
                if (typeof trs === "undefined") return

                // Loop through stats
                for (let idx = 0; idx < payload.stats.length; idx ++) {
                    // Line doesn't exist
                    if (trs.length <= idx) break

                    // Get rows
                    const tds = trs[idx].querySelectorAll("td")

                    // Not enough rows
                    if (tds.length < 2) continue

                    // Set value
                    tds[1].querySelector("span").innerText = payload.stats[idx].value.toFixed(2)
                }
                break
        }
    },
}