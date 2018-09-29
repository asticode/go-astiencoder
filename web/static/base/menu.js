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
        if (typeof data.workflows !== "undefined") {
            for (let k = 0; k < data.workflows.length; k++) {
                menu.addWorkflow(data.workflows[k])
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
            is_stopped: data.is_stopped,
            name: data.name,
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
        r.html.toggle.className = "toggle " + (data.is_stopped ? "off": "on")
        r.html.toggle.onclick = function() {
            // TODO Handle toggle click
        }
        toggleCell.appendChild(r.html.toggle)

        // Create slider
        const slider = document.createElement("span")
        slider.className = "slider"
        r.html.toggle.appendChild(slider)
        return r
    },
}