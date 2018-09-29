let menu = {
    workflows: {},

    init: function(data) {
        if (typeof data.workflows !== "undefined") {
            for (let k = 0; k < data.workflows.length; k++) {
                menu.addWorkflow(data.workflows[k])
            }
        }
    },
    addWorkflow: function(data) {
        // Workflow already exists
        if (typeof menu.workflows[data.name] !== "undefined") {
            return
        }

        // Create workflow
        let workflow = menu.newWorkflow(data)

        // Add in alphabetical order
        asticode.tools.appendSorted(document.getElementById("menu"), workflow, menu.workflows)

        // Append to pool
        this.workflows[workflow.name] = workflow
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
        r.html.wrapper.className = "menu-workflow"
        r.html.wrapper.innerHTML = "<a href='/web/workflows?name=" + encodeURIComponent(data.name) + "'>" + data.name + "</a>"
        return r
    },
}