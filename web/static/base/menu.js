let menu = {
    workflows: {
        html: {},
        pool: {},
    },

    init: function(data) {
        // Create workflow title
        this.workflows.html.title = document.createElement("div")
        this.workflows.html.title.className = "menu-title"
        this.workflows.html.title.innerText = "Workflows"
        this.workflows.html.title.style.display = "none"
        document.getElementById("menu").appendChild(this.workflows.html.title)

        // Create workflow table
        this.workflows.html.table = document.createElement("div")
        this.workflows.html.table.className = "table"
        document.getElementById("menu").appendChild(this.workflows.html.table)

        // Add workflows
        if (typeof data !== "undefined") {
            for (let k = 0; k < data.length; k++) {
                menu.addWorkflow(data[k])
            }
        }
    },
    addWorkflow: function(data) {
        // Workflow already exists
        if (typeof this.workflows.pool[data.name] !== "undefined") {
            return
        }

        // Create workflow
        const workflow = this.newWorkflow(data)

        // Add in alphabetical order
        asticode.tools.appendSorted(this.workflows.html.table, workflow, this.workflows.pool)

        // Append to pool
        this.workflows.pool[workflow.name] = workflow

        // Show title
        if (Object.keys(this.workflows.pool).length > 0) this.workflows.html.title.style.display = "block"
    },
    newWorkflow: function(data) {
        // Init
        let r = {
            html: {},
            name: data.name,
            status: data.status,
        }

        // Create wrapper
        r.html.wrapper = document.createElement("div")
        r.html.wrapper.className = "row"

        // Create name
        const name = document.createElement("div")
        name.className = "cell"
        name.style.paddingRight = "10px"
        name.innerHTML = "<a href='/web/workflow?name=" + encodeURIComponent(data.name) + "'>" + data.name + "</a>"
        r.html.wrapper.appendChild(name)

        // Create toggle cell
        const toggleCell = document.createElement("div")
        toggleCell.className = "cell"
        r.html.wrapper.appendChild(toggleCell)

        // Create toggle
        r.html.toggle = document.createElement("label")
        r.html.toggle.className = "toggle " + (data.status === "running" ? "on": "off")
        toggleCell.appendChild(r.html.toggle)

        // Create slider
        const slider = document.createElement("span")
        slider.className = "slider"
        slider.onclick = function() {
            let action = "start"
            switch (menu.workflows.pool[data.name].status) {
                case "running":
                    action = "pause"
                    break
                case "paused":
                    action = "continue"
                    break
            }
            asticode.tools.sendHttp({
                method: "GET",
                url: "/api/workflows/" + data.name + "/" + action,
                error: base.defaultHttpError,
            })
        }
        r.html.toggle.appendChild(slider)
        return r
    },
    websocketFunc: function(eventName, payload) {
        switch (eventName) {
            case "workflow.continued":
                this.updateToggle(payload, "running")
                break
            case "workflow.paused":
                this.updateToggle(payload, "paused")
                break
            case "workflow.started":
                this.updateToggle(payload, "running")
                break
            case "workflow.stopped":
                this.updateToggle(payload, "stopped")
                break
        }
    },
    updateToggle: function(name, status) {
        // Fetch workflow
        const workflow = this.workflows.pool[name];

        // Workflow doesn't exist
        if (typeof workflow === "undefined") return

        // Update class
        asticode.tools.removeClass(workflow.html.toggle, status === "running" ? "off" : "on")
        asticode.tools.addClass(workflow.html.toggle, status === "running" ? "on" : "off")

        // Update attribute
        workflow.status = status
    },
}